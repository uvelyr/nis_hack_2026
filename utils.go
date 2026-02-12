package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
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
	"golang.org/x/crypto/bcrypt"
)

var db *gorm.DB
var waClient *whatsmeow.Client
var jwtKey = []byte("alertmen_secret_key_2026_top_secret")

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open("alertmen.db"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.AutoMigrate(&User{}, &Channel{}, &Subscription{}, &Notification{}, &Report{})
}

func initWhatsApp() {
	dbLog := waLog.Stdout("Database", "ERROR", true)
	// ИСПРАВЛЕНО: Добавлен context.Background() в начало
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:whatsapp_session.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("WhatsApp", "ERROR", true)
	waClient = whatsmeow.NewClient(deviceStore, clientLog)

	if waClient.Store.ID == nil {
		qrChan, _ := waClient.GetQRChannel(context.Background())
		err = waClient.Connect()
		if err != nil { panic(err) }
		
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("\n--- SCAN QR CODE ---")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			}
		}
	} else {
		waClient.Connect()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func register(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil { return }
	h, _ := bcrypt.GenerateFromPassword([]byte(input.Password), 10)
	user := User{Username: input.Username, Password: string(h)}
	db.Create(&user)
	c.JSON(201, gin.H{"status": "created"})
}

func createReport(c *gin.Context) {
	uid, _ := c.Get("userID")
	var r Report
	if err := c.ShouldBindJSON(&r); err != nil { return }
	r.SenderID = uid.(uint)
	r.Status = "pending"
	db.Create(&r)
	c.JSON(201, gin.H{"status": "pending"})
}

func getNotifications(c *gin.Context) {
	uid, _ := c.Get("userID")
	var n []Notification
	db.Where("user_id = ?", uid).Order("created_at desc").Find(&n)
	c.JSON(200, n)
}

func subscribe(c *gin.Context) {
	uid, _ := c.Get("userID")
	var s Subscription
	c.ShouldBindJSON(&s)
	s.UserID = uid.(uint)
	db.Save(&s)
	c.JSON(200, gin.H{"status": "ok"})
}

func updatePhone(c *gin.Context) {
	uid, _ := c.Get("userID")
	var input struct{ Phone string `json:"phone"` }
	c.ShouldBindJSON(&input)
	db.Model(&User{}).Where("id = ?", uid).Update("phone", input.Phone)
	c.JSON(200, gin.H{"status": "ok"})
}

func sendWhatsAppMessage(phone, title, content string) {
	if waClient == nil { return }
	var clean string
	for _, r := range phone { if unicode.IsDigit(r) { clean += string(r) } }
	if strings.HasPrefix(clean, "8") { clean = "7" + clean[1:] }
	target := types.NewJID(clean, types.DefaultUserServer)
	
	msg := &waProto.Message{Conversation: proto.String(fmt.Sprintf("*%s*\n%s", title, content))}
	waClient.SendMessage(context.Background(), target, msg)
}

func seedChannels() {
	chs := []Channel{
		{Title: "Channel 1", Slug: "ch1", ModeratorID: 1},
		{Title: "Channel 2", Slug: "ch2", ModeratorID: 1},
	}
	for _, ch := range chs {
		db.Where(Channel{Slug: ch.Slug}).FirstOrCreate(&ch)
	}
}
