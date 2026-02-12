package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// --- МОДЕЛИ ДАННЫХ ---

type User struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Username      string    `gorm:"unique;not null" json:"username"`
	Password      string    `json:"-"`
	Phone         string    `json:"phone"`          // Номер для рассылки (например, 77071234567)
	TelegramID    string    `json:"telegram_id"`    // Пока игнорируем, но поле оставляем
	TelegramToken string    `json:"telegram_token"` // Хэш для будущей привязки ТГ
	CreatedAt     time.Time `json:"created_at"`
}

type Category struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"` // incident, weather и т.д.
}

type Subscription struct {
	UserID       uint `gorm:"primaryKey" json:"user_id"`
	CategoryID   uint `gorm:"primaryKey" json:"category_id"`
	SendWhatsApp bool `json:"send_whatsapp" gorm:"default:false"`
	SendTelegram bool `json:"send_telegram" gorm:"default:false"`
}

type Notification struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `json:"user_id"`
	CategoryID uint      `json:"category_id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

// --- СЛУЖЕБНЫЕ ФУНКЦИИ ОТПРАВКИ ---

func sendWhatsAppMessage(phone, title, content string) {
    if waClient == nil {
        fmt.Println("WhatsApp client is NIL")
        return
    }
    if !waClient.IsConnected() {
        fmt.Println("WhatsApp client is NOT connected")
        return
    }

    // 1. Clean the phone
    var cleanPhone string
    for _, r := range phone {
        if unicode.IsDigit(r) {
            cleanPhone += string(r)
        }
    }

    // CIS fix: 8... -> 7...
    if strings.HasPrefix(cleanPhone, "8") && len(cleanPhone) == 11 {
        cleanPhone = "7" + cleanPhone[1:]
    }

    // 2. Build JID - explicitly use the user server
    // For many regions, it's essential to ensure the JID is correct
    targetJID := types.NewJID(cleanPhone, types.DefaultUserServer)
    
    fmt.Printf("Attempting to send WA to: %s (JID: %s)\n", cleanPhone, targetJID.String())

    formattedText := fmt.Sprintf("*%s*\n\n%s\n\n_Alertmen Service_", strings.ToUpper(title), content)

    msg := &waProto.Message{
        Conversation: proto.String(formattedText),
    }

    // 3. Send and WAIT for error (don't ignore the result)
    resp, err := waClient.SendMessage(context.Background(), targetJID, msg)
    if err != nil {
        fmt.Printf("WA Error for %s: %v\n", cleanPhone, err)
    } else {
        fmt.Printf("WA Success! Message ID: %s sent at %v\n", resp.ID, resp.Timestamp)
    }
}

// --- ХЕНДЛЕРЫ API ---

func register(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Введите логин и пароль"})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	user := User{Username: input.Username, Password: string(hash)}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Логин уже занят"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user_id": user.ID, "message": "Аккаунт Alertmen создан"})
}

func login(c *gin.Context) {
    var input struct {
        Username string `json:"username" binding:"required"`
        Password string `json:"password" binding:"required"`
    }

    if err := c.ShouldBindJSON(&input); err != nil {
        // This will tell us WHY it's failing to read the username
        c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed JSON", "details": err.Error()})
        return
    }

    fmt.Printf("Attempting login for: [%s]\n", input.Username) // Verify this isn't empty!

    var user User
    if err := db.Where("username = ?", input.Username).First(&user).Error; err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
        return
    }

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный логин или пароль"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "username": user.Username})
}

func updatePhone(c *gin.Context) {
	var input struct {
		UserID uint   `json:"user_id" binding:"required"`
		Phone  string `json:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID и номер телефона обязательны"})
		return
	}

	db.Model(&User{}).Where("id = ?", input.UserID).Update("phone", input.Phone)
	c.JSON(http.StatusOK, gin.H{"message": "Контактные данные обновлены"})
}

func subscribe(c *gin.Context) {
	var input struct {
		UserID       uint `json:"user_id" binding:"required"`
		CategoryID   uint `json:"category_id" binding:"required"`
		SendWhatsApp bool `json:"send_whatsapp"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка в параметрах подписки"})
		return
	}

	sub := Subscription{
		UserID:       input.UserID,
		CategoryID:   input.CategoryID,
		SendWhatsApp: input.SendWhatsApp,
	}

	db.Save(&sub)
	c.JSON(http.StatusOK, gin.H{"status": "Настройки подписки сохранены"})
}

func externalWebhook(c *gin.Context) {
	var input struct {
		Type    string `json:"type" binding:"required"` // тип категории (например, incident)
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Данные неполные"})
		return
	}

	var cat Category
	if err := db.Where("type = ?", input.Type).First(&cat).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Категория не найдена"})
		return
	}

	var subs []Subscription
	db.Where("category_id = ?", cat.ID).Find(&subs)

	for _, sub := range subs {
		// Сохраняем уведомление в БД (личная история)
		db.Create(&Notification{
			UserID:     sub.UserID,
			CategoryID: cat.ID,
			Title:      input.Title,
			Content:    input.Content,
			CreatedAt:  time.Now(),
		})

		// Если включен WhatsApp, отправляем
		if sub.SendWhatsApp {
			var user User
			db.First(&user, sub.UserID)
			if user.Phone != "" {
				// Запускаем в горутине, чтобы не тормозить цикл
				go sendWhatsAppMessage(user.Phone, input.Title, input.Content)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "Success",
		"message":  "Уведомления Alertmen обработаны",
		"notified": len(subs),
	})
}

// --- MAIN ---

func main() {
	// Инициализация из utils.go
	initDB()
	seedCategories()
    initWhatsApp()    

	r := gin.Default()
	r.Use(CORSMiddleware())

	api := r.Group("/api")
	{
		api.POST("/register", register)
		api.POST("/login", login)
		api.POST("/profile/phone", updatePhone)
		api.POST("/subscribe", subscribe)
		api.POST("/webhook/send", externalWebhook)
		
		api.GET("/categories", func(c *gin.Context) {
			var cats []Category
			db.Find(&cats)
			c.JSON(http.StatusOK, cats)
		})

		api.GET("/notifications/:id", func(c *gin.Context) {
			userID := c.Param("id")
			var notes []Notification
			db.Where("user_id = ?", userID).Order("created_at desc").Find(&notes)
			c.JSON(http.StatusOK, notes)
		})
	}

	fmt.Println("\n====================================")
	fmt.Println("   ALERTMEN BACKEND IS RUNNING")
	fmt.Println("   Port: 8080")
	fmt.Println("====================================\n")

	r.Run(":8080")
}
