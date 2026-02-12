package main

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// --- MODELS ---

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"unique;not null" json:"username"`
	Password  string    `json:"-"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
}

type Channel struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	ModeratorID uint   `json:"moderator_id"`
}

type Subscription struct {
	UserID       uint `gorm:"primaryKey;column:user_id" json:"user_id"`
	ChannelID    uint `gorm:"primaryKey;column:channel_id" json:"channel_id"`
	SendWhatsApp bool `gorm:"column:send_whatsapp;not null;default:false" json:"send_whatsapp"`
}

type Report struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SenderID  uint      `json:"sender_id"`
	ChannelID uint      `json:"channel_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	ImagePath string    `json:"image_path"`
	Status    string    `json:"status" gorm:"default:pending"`
	CreatedAt time.Time `json:"created_at"`
}

type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `json:"user_id"`
	ChannelID uint      `json:"channel_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	ImagePath string    `json:"image_path"`
	CreatedAt time.Time `json:"created_at"`
}

// --- MIDDLEWARES ---

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if tokenString == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "No token"})
			return
		}
		token, _ := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})
		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			c.Set("userID", uint(claims["user_id"].(float64)))
			c.Set("isModerator", claims["is_moderator"].(bool))
			c.Next()
		} else {
			c.AbortWithStatusJSON(401, gin.H{"error": "Invalid token"})
		}
	}
}

func ModeratorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		isMod, _ := c.Get("isModerator")
		if isMod != true {
			c.AbortWithStatusJSON(403, gin.H{"error": "Forbidden: Not a moderator"})
			return
		}
		c.Next()
	}
}

// --- HANDLERS ---

func login(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	c.ShouldBindJSON(&input)
	var user User
	if err := db.Where("username = ?", input.Username).First(&user).Error; err != nil {
		c.JSON(401, gin.H{"error": "User not found"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(401, gin.H{"error": "Wrong password"})
		return
	}
	var count int64
	db.Model(&Channel{}).Where("moderator_id = ?", user.ID).Count(&count)
	isMod := count > 0
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":      user.ID,
		"is_moderator": isMod,
		"exp":          time.Now().Add(time.Hour * 72).Unix(),
	})
	t, _ := token.SignedString(jwtKey)
	c.JSON(200, gin.H{"token": t, "user_id": user.ID, "is_moderator": isMod})
}

func getModeratorInbox(c *gin.Context) {
	userID, _ := c.Get("userID")
	var channelIDs []uint
	db.Model(&Channel{}).Where("moderator_id = ?", userID).Pluck("id", &channelIDs)
	var reports []Report
	db.Where("channel_id IN ? AND status = ?", channelIDs, "pending").Find(&reports)
	c.JSON(200, reports)
}

func moderateReport(c *gin.Context) {
	userID, _ := c.Get("userID")
	reportID := c.Param("id")
	action := c.Param("action")
	var report Report
	if err := db.First(&report, reportID).Error; err != nil {
		c.JSON(404, gin.H{"error": "Report not found"})
		return
	}
	var ch Channel
	db.First(&ch, report.ChannelID)
	if ch.ModeratorID != userID.(uint) {
		c.JSON(403, gin.H{"error": "Not your channel"})
		return
	}
	if action == "approve" {
		report.Status = "approved"
		db.Save(&report)
		var subs []Subscription
		db.Where("channel_id = ?", report.ChannelID).Find(&subs)
		for _, s := range subs {
			db.Create(&Notification{UserID: s.UserID, ChannelID: report.ChannelID, Title: report.Title, Content: report.Content, ImagePath: report.ImagePath, CreatedAt: time.Now()})
			if s.SendWhatsApp {
				var u User
				db.First(&u, s.UserID)
				go sendWhatsAppMessage(u.Phone, report.Title, report.Content, report.ImagePath)
			}
		}
		c.JSON(200, gin.H{"message": "Approved"})
	} else {
		report.Status = "rejected"
		db.Save(&report)
		c.JSON(200, gin.H{"message": "Rejected"})
	}
}

func manualWebhook(c *gin.Context) {
	userID, _ := c.Get("userID")
	var input struct {
		ChannelID uint   `json:"channel_id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		ImageURL  string `json:"image_url"`
	}
	c.ShouldBindJSON(&input)
	var ch Channel
	db.First(&ch, input.ChannelID)
	if ch.ModeratorID != userID.(uint) {
		c.JSON(403, gin.H{"error": "Forbidden"})
		return
	}
	var subs []Subscription
	db.Where("channel_id = ?", input.ChannelID).Find(&subs)
	for _, s := range subs {
		db.Create(&Notification{UserID: s.UserID, ChannelID: input.ChannelID, Title: input.Title, Content: input.Content, ImagePath: input.ImageURL, CreatedAt: time.Now()})
		if s.SendWhatsApp {
			var u User
			db.First(&u, s.UserID)
			go sendWhatsAppMessage(u.Phone, input.Title, input.Content, input.ImageURL)
		}
	}
	c.JSON(200, gin.H{"status": "sent"})
}

func getChannels(c *gin.Context) {
	var channels []Channel
	db.Find(&channels)
	c.JSON(200, channels)
}

func main() {
	initDB()
	initWhatsApp()
	seedChannels()
	r := gin.Default()
	r.Use(CORSMiddleware())
	r.Static("/uploads", "./uploads")
	api := r.Group("/api")
	{
		api.POST("/register", register)
		api.POST("/login", login)
		auth := api.Group("/")
		auth.Use(AuthMiddleware())
		{
			auth.GET("/channels", getChannels)
			auth.POST("/reports", createReport)
			auth.GET("/notifications", getNotifications)
			auth.POST("/subscribe", subscribe)
			auth.POST("/profile/phone", updatePhone)
			mod := auth.Group("/moderation")
			mod.Use(ModeratorMiddleware())
			{
				mod.GET("/inbox", getModeratorInbox)
				mod.POST("/:action/:id", moderateReport)
				mod.POST("/webhook", manualWebhook)
			}
		}
	}
	r.Run(":8080")
}
