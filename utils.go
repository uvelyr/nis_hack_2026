package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"github.com/gin-gonic/gin"
	"github.com/mdp/qrterminal/v3"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB
var waClient *whatsmeow.Client

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open("alertmen.db"), &gorm.Config{})
	if err != nil {
		panic("Ошибка БД: " + err.Error())
	}
	db.AutoMigrate(&User{}, &Category{}, &Subscription{}, &Notification{})
}

func initWhatsApp() {
	// 1. Создаем контекст для работы с БД и клиентом
	ctx := context.Background()

	// 2. Настраиваем логирование (только ошибки, чтобы не спамить в консоль)
	dbLog := waLog.Stdout("Database", "ERROR", true)

	// 3. Подключаем базу данных сессий WhatsApp. 
	// Используем WAL и busy_timeout, чтобы избежать блокировок SQLite.
	dbParams := "file:whatsapp_session.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"
	container, err := sqlstore.New(ctx, "sqlite3", dbParams, dbLog)
	if err != nil {
		panic(fmt.Errorf("не удалось инициализировать хранилище WhatsApp: %v", err))
	}

	// 5. Получаем или создаем устройство в базе
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(fmt.Errorf("не удалось получить данные устройства: %v", err))
	}

	// 6. Инициализируем клиент
	clientLog := waLog.Stdout("WhatsApp", "ERROR", true)
	waClient = whatsmeow.NewClient(deviceStore, clientLog)

	// 7. ФИКС СИНХРОНИЗАЦИИ: Отключаем автоматическую подгрузку истории сообщений.
	// Это экономит трафик и предотвращает массовую запись старых данных в БД.
	waClient.ManualHistorySyncDownload = true

	// 8. Проверка авторизации и подключение
	if waClient.Store.ID == nil {
		// Если сессии нет — запрашиваем QR-код
		qrChan, _ := waClient.GetQRChannel(ctx)
		err = waClient.Connect()
		if err != nil {
			panic(err)
		}
		
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("\n====================================")
				fmt.Println("   ALERTMEN: ПРИВЯЗКА WHATSAPP")
				fmt.Println("====================================")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("Отсканируйте код через WhatsApp -> Связанные устройства")
			} else {
				fmt.Println("Статус QR-события:", evt.Event)
			}
		}
	} else {
		// Если сессия есть — просто подключаемся
		err = waClient.Connect()
		if err != nil {
			panic(fmt.Errorf("ошибка подключения к WhatsApp: %v", err))
		}
		fmt.Println("Alertmen: Сессия WhatsApp успешно восстановлена.")
	}
}

func seedCategories() {
	// Список базовых категорий для старта
	categories := []Category{
		{Title: "Происшествия", Type: "incident"},
		{Title: "Погода", Type: "weather"},
		{Title: "Личные сообщения", Type: "whatsapp"},
	}

	for _, cat := range categories {
		var existing Category
		// Ищем категорию по типу, чтобы не дублировать
		result := db.Where("type = ?", cat.Type).First(&existing)
		
		if result.Error != nil {
			// Если не нашли (ошибка RecordNotFound), создаем
			db.Create(&cat)
			fmt.Printf("Категория [%s] добавлена в базу\n", cat.Title)
		}
	}
}

func generateRandomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

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
