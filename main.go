package main

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// --- МОДЕЛИ ДАННЫХ ---

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"unique;not null" json:"username"`
	Password  string    `json:"-"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role" gorm:"default:user"` // "user" или "moderator"
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
	Status    string    `json:"status" gorm:"default:pending"` // pending, approved, rejected
	CreatedAt time.Time `json:"created_at"`
}

type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `json:"user_id"`
	ChannelID uint      `json:"channel_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// --- ХЕНДЛЕРЫ АУТЕНТИФИКАЦИИ ---

func debugWebhook(c *gin.Context) {
    var input struct {
        Type    string `json:"type" binding:"required"`
        Title   string `json:"title"`
        Content string `json:"content"`
    }
    if err := c.ShouldBindJSON(&input); err != nil {
        c.JSON(400, gin.H{"error": "Bad JSON"})
        return
	}

    // В новой логике мы ищем канал по Slug (например "kzl_incident")
    var channel Channel
    if err := db.Where("slug = ?", input.Type).First(&channel).Error; err != nil {
        c.JSON(404, gin.H{"error": "Channel not found with slug: " + input.Type})
        return
    }

    // Рассылаем всем подписчикам этого канала напрямую (для теста)
    var subs []Subscription
    db.Where("channel_id = ? AND send_whatsapp = ?", channel.ID, 1).Find(&subs)
    
    for _, sub := range subs {
        var u User
        if err := db.First(&u, sub.UserID).Error; err == nil && u.Phone != "" {
            go sendWhatsAppMessage(u.Phone, input.Title, input.Content)
        }
    }

    c.JSON(200, gin.H{"status": "Debug broadcast sent", "recipients": len(subs)})
}

func register(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "Введите логин и пароль"})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	user := User{Username: input.Username, Password: string(hash), Role: "user"}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(409, gin.H{"error": "Логин уже занят"})
		return
	}
	c.JSON(201, gin.H{"message": "Аккаунт создан"})
}

func login(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "Ошибка входа"})
		return
	}

	var user User
	if err := db.Where("username = ?", input.Username).First(&user).Error; err != nil {
		c.JSON(401, gin.H{"error": "Пользователь не найден"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(401, gin.H{"error": "Неверный пароль"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"role":    user.Role,
		"exp":     time.Now().Add(time.Hour * 72).Unix(),
	})
	tokenString, _ := token.SignedString(jwtKey)

	c.JSON(200, gin.H{"token": tokenString, "user_id": user.ID, "role": user.Role})
}

// --- ХЕНДЛЕРЫ БИЗНЕС-ЛОГИКИ ---

func updatePhone(c *gin.Context) {
	userID, _ := c.Get("userID")
	var input struct {
		Phone string `json:"phone" binding:"required"`
	}
	c.ShouldBindJSON(&input)
	db.Model(&User{}).Where("id = ?", userID).Update("phone", input.Phone)
	c.JSON(200, gin.H{"message": "Телефон обновлен"})
}

func createReport(c *gin.Context) {
	userID, _ := c.Get("userID")
	var input struct {
		ChannelID uint   `json:"channel_id" binding:"required"`
		Title     string `json:"title" binding:"required"`
		Content   string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "Заполните все поля"})
		return
	}

	report := Report{
		SenderID:  userID.(uint),
		ChannelID: input.ChannelID,
		Title:     input.Title,
		Content:   input.Content,
	}
	db.Create(&report)

	// Уведомление модератору канала
	var channel Channel
	db.First(&channel, input.ChannelID)
	var moderator User
	if err := db.First(&moderator, channel.ModeratorID).Error; err == nil && moderator.Phone != "" {
		msg := fmt.Sprintf("⚠️ НОВЫЙ ОТЧЕТ в канале %s\nНужен аппрув!", channel.Title)
		go sendWhatsAppMessage(moderator.Phone, "МОДЕРАЦИЯ", msg)
	}

	c.JSON(201, gin.H{"message": "Отчет отправлен на модерацию"})
}

func approveReport(c *gin.Context) {
	userID, _ := c.Get("userID")
	reportID := c.Param("id")

	var report Report
	if err := db.First(&report, reportID).Error; err != nil {
		c.JSON(404, gin.H{"error": "Отчет не найден"})
		return
	}

	// Проверяем, является ли текущий юзер модератором этого канала
	var channel Channel
	db.First(&channel, report.ChannelID)
	if channel.ModeratorID != userID.(uint) {
		c.JSON(403, gin.H{"error": "Вы не модератор этого канала"})
		return
	}

	report.Status = "approved"
	db.Save(&report)

	// Рассылка подписчикам канала
	var subs []Subscription
	db.Where("channel_id = ? AND send_whatsapp = ?", report.ChannelID, true).Find(&subs)
	for _, sub := range subs {
		var u User
		if err := db.First(&u, sub.UserID).Error; err == nil && u.Phone != "" {
			go sendWhatsAppMessage(u.Phone, report.Title, report.Content)
		}
	}

	c.JSON(200, gin.H{"status": "Одобрено и разослано"})
}

// --- MAIN ---

func main() {
	initDB()
	initWhatsApp()
	seedChannels()

	r := gin.Default()
	r.Use(CORSMiddleware())

	api := r.Group("/api")
	{
		// Public routes
		api.POST("/register", register)
		api.POST("/login", login)
		api.POST("/webhook/send", debugWebhook)

		// Protected routes
		auth := api.Group("/")
		auth.Use(AuthMiddleware())
		{
			auth.POST("/profile/phone", updatePhone)
			auth.POST("/reports", createReport)
			auth.POST("/moderation/approve/:id", approveReport)

			auth.GET("/channels", func(c *gin.Context) {
				var chs []Channel
				db.Find(&chs)
				c.JSON(200, chs)
			})

			auth.POST("/subscribe", func(c *gin.Context) {
				uid, _ := c.Get("userID")

				var input struct {
					ChannelID    uint `json:"channel_id"`
					SendWhatsApp bool `json:"send_whatsapp"`
				}

				if err := c.ShouldBindJSON(&input); err != nil {
					c.JSON(400, gin.H{"error": "Invalid JSON: " + err.Error()})
					return
				}

				sub := Subscription{
					UserID:       uid.(uint),
					ChannelID:    input.ChannelID,
					SendWhatsApp: input.SendWhatsApp,
				}

				// Теперь логика сохранения находится внутри хендлера
				if err := db.Save(&sub).Error; err != nil {
					c.JSON(500, gin.H{"error": "Database error: " + err.Error()})
					return
				}

				c.JSON(200, gin.H{
					"status":  "ok",
					"channel": input.ChannelID,
					"wa":      input.SendWhatsApp,
				})
			})
		}
	}

	r.Run(":8080")
}
