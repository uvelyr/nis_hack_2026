package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// --- МОДЕЛИ ДАННЫХ ---

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"unique;not null" json:"username"`
	Password  string    `json:"-"`
	WAFilter  string    `json:"wa_filter"`
	CreatedAt time.Time `json:"created_at"`
}

type Category struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"` // incident, weather, whatsapp
}

type Subscription struct {
	UserID     uint `gorm:"primaryKey" json:"user_id"`
	CategoryID uint `gorm:"primaryKey" json:"category_id"`
}

type Notification struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `json:"user_id"`
	CategoryID uint      `json:"category_id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

var db *gorm.DB

// Инициализация базы данных
func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open("notify.db"), &gorm.Config{})
	if err != nil {
		panic("Не удалось подключить базу данных: " + err.Error())
	}

	db.AutoMigrate(&User{}, &Category{}, &Subscription{}, &Notification{})

	var count int64
	db.Model(&Category{}).Count(&count)
	if count == 0 {
		initialCategories := []Category{
			{Title: "Происшествия в г. Кызылорда", Type: "incident"},
			{Title: "Погода г. Кызылорда", Type: "weather"},
			{Title: "Сообщения WhatsApp", Type: "whatsapp"},
		}
		db.Create(&initialCategories)
	}
}

// --- ХЕНДЛЕРЫ ---

func register(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных"})
		return
	}

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	user := User{Username: input.Username, Password: string(hashedPassword)}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Такой логин уже занят"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Аккаунт создан", "user_id": user.ID})
}

func login(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Введите логин и пароль"})
		return
	}

	var user User
	if err := db.Where("username = ?", input.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный логин или пароль"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный логин или пароль"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Успешный вход", "user_id": user.ID})
}

func getCategories(c *gin.Context) {
	var categories []Category
	db.Find(&categories)
	c.JSON(http.StatusOK, categories)
}

func subscribe(c *gin.Context) {
	var input struct {
		UserID     uint `json:"user_id" binding:"required"`
		CategoryID uint `json:"category_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Нужны ID пользователя и категории"})
		return
	}

	sub := Subscription{UserID: input.UserID, CategoryID: input.CategoryID}
	db.FirstOrCreate(&sub)
	c.JSON(http.StatusOK, gin.H{"status": "Подписка сохранена"})
}

func setupWhatsApp(c *gin.Context) {
	userID := c.Query("user_id")
	filter := c.Query("filter")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id обязателен"})
		return
	}

	db.Model(&User{}).Where("id = ?", userID).Update("wa_filter", filter)

	c.JSON(http.StatusOK, gin.H{
		"qr_code": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA...",
		"message": "QR-код сгенерирован. Фильтр установлен: " + filter,
	})
}

func getNotifications(c *gin.Context) {
	userID := c.Param("id")
	var notifications []Notification
	db.Where("user_id = ?", userID).Order("created_at desc").Find(&notifications)
	c.JSON(http.StatusOK, notifications)
}

func deleteNotification(c *gin.Context) {
	id := c.Param("id")
	db.Delete(&Notification{}, id)
	c.JSON(http.StatusOK, gin.H{"message": "Удалено"})
}

func externalWebhook(c *gin.Context) {
	var input struct {
		Type    string `json:"type" binding:"required"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Данные неполные"})
		return
	}

	var cat Category
	db.Where("type = ?", input.Type).First(&cat)

	var subs []Subscription
	db.Where("category_id = ?", cat.ID).Find(&subs)

	for _, sub := range subs {
		var user User
		db.First(&user, sub.UserID)

		if input.Type == "whatsapp" && user.WAFilter != "" {
			if !strings.Contains(strings.ToLower(input.Content), strings.ToLower(user.WAFilter)) {
				continue
			}
		}

		db.Create(&Notification{
			UserID:     user.ID,
			CategoryID: cat.ID,
			Title:      input.Title,
			Content:    input.Content,
			CreatedAt:  time.Now(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"status": "Уведомления обработаны"})
}

// --- MAIN С ЗАПЛАТКАМИ ДЛЯ NGORK И FRONTEND ---

func main() {
	initDB()

	r := gin.Default()

	// Middleware для CORS и обхода предупреждения ngrok
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, ngrok-skip-browser-warning")
		
		// Заголовок, чтобы ngrok не показывал страницу-заглушку
		c.Writer.Header().Set("ngrok-skip-browser-warning", "69420")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api")
	{
		api.POST("/register", register)
		api.POST("/login", login)
		api.GET("/categories", getCategories)
		api.POST("/subscribe", subscribe)
		api.GET("/notifications/:id", getNotifications)
		api.DELETE("/notifications/:id", deleteNotification)
		api.POST("/webhook/send", externalWebhook)
	}

	fmt.Println("Сервер запущен на http://localhost:8080")
	r.Run(":8080")
}
