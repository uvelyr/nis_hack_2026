package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mdp/qrterminal/v3"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Глобальные переменные
var db *gorm.DB
var waClient *whatsmeow.Client
var jwtKey = []byte("alertmen_secret_key_2026_top_secret")

// --- ИНИЦИАЛИЗАЦИЯ ИНФРАСТРУКТУРЫ ---

func initDB() {
    var err error
    db, err = gorm.Open(sqlite.Open("alertmen.db"), &gorm.Config{})
    if err != nil {
        panic("Ошибка БД: " + err.Error())
    }
    // Убираем Category, добавляем Channel и Report
    db.AutoMigrate(&User{}, &Channel{}, &Subscription{}, &Notification{}, &Report{})
}

func initWhatsApp() {
	ctx := context.Background()
	dbLog := waLog.Stdout("Database", "ERROR", true)
	dbParams := "file:whatsapp_session.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"
	
	container, err := sqlstore.New(ctx, "sqlite3", dbParams, dbLog)
	if err != nil {
		panic(fmt.Errorf("не удалось инициализировать хранилище WhatsApp: %v", err))
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(fmt.Errorf("не удалось получить данные устройства: %v", err))
	}

	clientLog := waLog.Stdout("WhatsApp", "ERROR", true)
	waClient = whatsmeow.NewClient(deviceStore, clientLog)
	waClient.ManualHistorySyncDownload = true

	if waClient.Store.ID == nil {
		qrChan, _ := waClient.GetQRChannel(ctx)
		err = waClient.Connect()
		if err != nil {
			panic(err)
		}
		
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("\n--- ALERTMEN: SCAN QR CODE ---")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			}
		}
	} else {
		err = waClient.Connect()
		if err != nil {
			panic(fmt.Errorf("ошибка подключения: %v", err))
		}
		fmt.Println("Alertmen: WhatsApp сессия активна.")
	}
}

// --- MIDDLEWARE ---

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, ngrok-skip-browser-warning")
		c.Writer.Header().Set("ngrok-skip-browser-warning", "69420")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Format: Bearer {token}"})
			return
		}

		token, err := jwt.Parse(parts[1], func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		claims, _ := token.Claims.(jwt.MapClaims)
		c.Set("userID", uint(claims["user_id"].(float64)))
		c.Next()
	}
}

// --- СЛУЖЕБНЫЕ ФУНКЦИИ ---

func sendWhatsAppMessage(phone, title, content string) {
	if waClient == nil || !waClient.IsConnected() {
		return
	}

	var cleanPhone string
	for _, r := range phone {
		if unicode.IsDigit(r) {
			cleanPhone += string(r)
		}
	}
	if strings.HasPrefix(cleanPhone, "8") && len(cleanPhone) == 11 {
		cleanPhone = "7" + cleanPhone[1:]
	}

	targetJID := types.NewJID(cleanPhone, types.DefaultUserServer)
	formattedText := fmt.Sprintf("*🔔 %s*\n\n%s\n\n_Alertmen Service_", strings.ToUpper(title), content)

	msg := &waProto.Message{Conversation: proto.String(formattedText)}
	waClient.SendMessage(context.Background(), targetJID, msg)
}

func generateRandomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func seedChannels() {
    channels := []Channel{
        {Title: "Кызылорда: Происшествия", Slug: "kzl_incidents", ModeratorID: 1},
        {Title: "Кызылорда: Погода", Slug: "kzl_weather", ModeratorID: 1},
        {Title: "Общий канал", Slug: "global", ModeratorID: 1},
    }

    for _, ch := range channels {
        var existing Channel
        // Ищем по Slug, чтобы не дублировать при каждом запуске
        if err := db.Where("slug = ?", ch.Slug).First(&existing).Error; err != nil {
            db.Create(&ch)
            fmt.Printf("Канал [%s] создан\n", ch.Title)
        }
    }
}
