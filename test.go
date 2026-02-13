package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	baseURL     string
	scanner     = bufio.NewScanner(os.Stdin)
	authToken   string
	isModerator bool
)

func main() {
	// Настройка адреса
	fmt.Println("=== ALERTMEN CONFIG ===")
	fmt.Print("Введите адрес сервера (например, localhost:8080 или ngrok-url.app): ")
	scanner.Scan()
	host := strings.TrimSpace(scanner.Text())

	if host == "" {
		host = "localhost:8080"
	}

	// Формируем правильный baseURL
	if !strings.HasPrefix(host, "http") {
		baseURL = "http://" + host
	} else {
		baseURL = host
	}
	
	// Убеждаемся, что в конце есть /api, но нет лишних слэшей
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/api") {
		baseURL += "/api"
	}

	fmt.Printf("Base URL установлен: %s\n", baseURL)

	for {
		fmt.Println("\n--- ALERTMEN TESTER ---")
		if authToken == "" {
			fmt.Println("1. Register | 2. Login | q. Quit")
		} else {
			fmt.Printf("Logged in (Moderator: %v)\n", isModerator)
			fmt.Println("3. List Channels | 4. Subscribe | 5. Send Report (with Image)")
			fmt.Println("6. Notifications History | 7. Set Phone Number")
			if isModerator {
				fmt.Println("8. [MOD] View Inbox | 9. [MOD] Approve | 10. [MOD] Reject | 11. [MOD] Webhook")
			}
			fmt.Println("0. Logout | q. Quit")
		}
		fmt.Print(">> ")
		scanner.Scan()
		input := scanner.Text()

		switch input {
		case "1": auth("register")
		case "2": auth("login")
		case "3": getRequest("/channels")
		case "4": subscribe()
		case "5": createReportMultipart()
		case "6": getRequest("/notifications")
		case "7": setPhone()
		case "8": if isModerator { viewInbox() }
		case "9": if isModerator { moderate("approve") }
		case "10": if isModerator { moderate("reject") }
		case "11": 
			if isModerator {
				webhook()
			}
		case "0": authToken = ""; isModerator = false
		case "q": return
		}
	}
}

// --- ФУНКЦИИ ОБРАБОТКИ ---

func auth(mode string) {
	fmt.Print("Username: "); scanner.Scan(); u := scanner.Text()
	fmt.Print("Password: "); scanner.Scan(); p := scanner.Text()
	data, code := postRequest("/"+mode, map[string]string{"username": u, "password": p})
	fmt.Printf("[%d] Response: %s\n", code, string(data))
	
	if code == 200 {
		var res map[string]interface{}
		json.Unmarshal(data, &res)
		if t, ok := res["token"].(string); ok {
			authToken = t
			isModerator = res["is_moderator"].(bool)
		}
	}
}

func setPhone() {
	fmt.Print("Enter Phone (digits only): ")
	scanner.Scan()
	p := scanner.Text()
	data, code := postRequest("/profile/phone", map[string]string{"phone": p})
	fmt.Printf("[%d] Response: %s\n", code, string(data))
}

func createReportMultipart() {
	fmt.Print("CH ID: "); scanner.Scan(); chID := scanner.Text()
	fmt.Print("Title: "); scanner.Scan(); title := scanner.Text()
	fmt.Print("Content: "); scanner.Scan(); content := scanner.Text()
	fmt.Print("Image Path (e.g. ./img.jpg): "); scanner.Scan(); imgPath := scanner.Text()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	writer.WriteField("channel_id", chID)
	writer.WriteField("title", title)
	writer.WriteField("content", content)

	if imgPath != "" {
		file, err := os.Open(imgPath)
		if err == nil {
			defer file.Close()
			part, _ := writer.CreateFormFile("image", filepath.Base(imgPath))
			io.Copy(part, file)
		} else {
			fmt.Println("File error, skipping image.")
		}
	}
	writer.Close()

	req, _ := http.NewRequest("POST", baseURL+"/reports", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addHeaders(req)

	resp, err := (&http.Client{}).Do(req)
	if err != nil { fmt.Println("Request failed"); return }
	defer resp.Body.Close()
	
	respData, _ := io.ReadAll(resp.Body)
	fmt.Printf("[%d] %s\n", resp.StatusCode, string(respData))
}

func viewInbox() {
	data, _ := getRequestData("/moderation/inbox")
	var reports []map[string]interface{}
	json.Unmarshal(data, &reports)
	fmt.Println("\n--- PENDING REPORTS ---")
	for _, r := range reports {
		fmt.Printf("ID: %v | Title: %s | Image: %v\n", r["id"], r["title"], r["image_path"])
	}
}

func moderate(action string) {
	fmt.Print("Report ID: "); scanner.Scan(); id := scanner.Text()
	_, code := postRequest("/moderation/"+action+"/"+id, nil)
	fmt.Printf("Result: %d\n", code)
}

func webhook() {
	fmt.Print("CH ID: "); scanner.Scan(); id, _ := strconv.Atoi(scanner.Text())
	fmt.Print("Title: "); scanner.Scan(); t := scanner.Text()
	fmt.Print("Content: "); scanner.Scan(); c := scanner.Text()
	postRequest("/moderation/webhook", map[string]interface{}{
		"channel_id": id, 
		"title": t, 
		"content": c,
	})
}

func subscribe() {
	fmt.Print("CH ID: "); scanner.Scan(); id, _ := strconv.Atoi(scanner.Text())
	postRequest("/subscribe", map[string]interface{}{"channel_id": id, "send_whatsapp": true})
}

// --- СЕРВИСНЫЕ ФУНКЦИИ ---

func addHeaders(req *http.Request) {
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	// Важно для ngrok, чтобы не вылетала страница-заглушка
	req.Header.Set("ngrok-skip-browser-warning", "true")
}

func postRequest(path string, payload interface{}) ([]byte, int) {
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req, _ := http.NewRequest("POST", baseURL+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	addHeaders(req)

	resp, err := (&http.Client{}).Do(req)
	if err != nil { return nil, 500 }
	defer resp.Body.Close()
	resBody, _ := io.ReadAll(resp.Body)
	return resBody, resp.StatusCode
}

func getRequestData(path string) ([]byte, int) {
	req, _ := http.NewRequest("GET", baseURL+path, nil)
	addHeaders(req)
	resp, err := (&http.Client{}).Do(req)
	if err != nil { return nil, 500 }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

func getRequest(path string) {
	d, c := getRequestData(path)
	fmt.Printf("[%d] %s\n", c, string(d))
}
