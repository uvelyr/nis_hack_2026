package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"golang.org/x/crypto/bcrypt"
	_ "github.com/mattn/go-sqlite3"
)

var db *gorm.DB
var waClient *whatsmeow.Client
var jwtKey = []byte("alertmen_secret_key_2026")

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open("alertmen.db"), &gorm.Config{})
	if err != nil { panic(err) }
	db.AutoMigrate(&User{}, &Channel{}, &Subscription{}, &Notification{}, &Report{})
	os.MkdirAll("./uploads", os.ModePerm)
}

func initWhatsApp() {
	dbLog := waLog.Stdout("Database", "ERROR", true)
	
	// Исправлено: добавлен context.Background() первым аргументом
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:whatsapp.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(fmt.Errorf("failed to connect to database: %w", err))
	}

	// Исправлено: добавлен context.Background() в вызов устройства
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		panic(fmt.Errorf("failed to get device: %w", err))
	}

	clientLog := waLog.Stdout("Client", "ERROR", true)
	waClient = whatsmeow.NewClient(device, clientLog)

	if waClient.Store.ID == nil {
		qrChan, _ := waClient.GetQRChannel(context.Background())
		err = waClient.Connect()
		if err != nil { panic(err) }
		
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("\n--- SCAN THIS QR CODE ---")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			}
		}
	} else {
		err = waClient.Connect()
		if err != nil { panic(err) }
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		if c.Request.Method == "OPTIONS" { c.AbortWithStatus(204); return }
		c.Next()
	}
}

func register(c *gin.Context) {
	var i struct{ Username, Password string }
	c.ShouldBindJSON(&i)
	h, _ := bcrypt.GenerateFromPassword([]byte(i.Password), 10)
	db.Create(&User{Username: i.Username, Password: string(h)})
	c.JSON(201, gin.H{"status": "ok"})
}

func createReport(c *gin.Context) {
	uid, _ := c.Get("userID")
	file, _ := c.FormFile("image")
	path := ""
	if file != nil {
		path = "uploads/" + file.Filename
		c.SaveUploadedFile(file, path)
	}
	chID, _ := strconv.Atoi(c.PostForm("channel_id"))
	db.Create(&Report{SenderID: uid.(uint), Title: c.PostForm("title"), Content: c.PostForm("content"), ChannelID: uint(chID), ImagePath: path})
	c.JSON(201, gin.H{"status": "pending"})
}

func getNotifications(c *gin.Context) {
	uid, _ := c.Get("userID")
	var n []Notification
	db.Where("user_id = ?", uid).Find(&n)
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
	var i struct{ Phone string }
	c.ShouldBindJSON(&i)
	db.Model(&User{}).Where("id = ?", uid).Update("phone", i.Phone)
	c.JSON(200, gin.H{"status": "ok"})
}

func sendWhatsAppMessage(phone, title, content, imgPath string) {
	if waClient == nil || phone == "" { return }
	var clean string
	for _, r := range phone { if unicode.IsDigit(r) { clean += string(r) } }
	target := types.NewJID(clean, types.DefaultUserServer)
	caption := fmt.Sprintf("*%s*\n%s", title, content)
	var msg *waProto.Message
	if imgPath != "" {
		data, _ := os.ReadFile(imgPath)
		upload, _ := waClient.Upload(context.Background(), data, whatsmeow.MediaImage)
		msg = &waProto.Message{ImageMessage: &waProto.ImageMessage{
			Caption: &caption, Mimetype: proto.String("image/jpeg"), 
			URL: &upload.URL, DirectPath: &upload.DirectPath, MediaKey: upload.MediaKey,
			FileEncSHA256: upload.FileEncSHA256, FileSHA256: upload.FileSHA256, FileLength: &upload.FileLength,
		}}
	} else {
		msg = &waProto.Message{Conversation: &caption}
	}
	waClient.SendMessage(context.Background(), target, msg)
}

func seedChannels() {
	db.FirstOrCreate(&Channel{Title: "City News", Slug: "city", ModeratorID: 1})
}
