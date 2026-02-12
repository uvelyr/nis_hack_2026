package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var baseURL string
var scanner = bufio.NewScanner(os.Stdin)
var currentUserID uint = 0
var authToken string = "" // Храним JWT здесь
var userRole string = ""

func main() {
	fmt.Print("Enter server IP (0 for localhost:8080): ")
	scanner.Scan()
	inputIP := strings.TrimSpace(scanner.Text())
	if inputIP == "0" || inputIP == "" {
		baseURL = "http://localhost:8080/api"
	} else {
		baseURL = "http://" + inputIP + ":8080/api"
	}

	fmt.Println("\n--- ALERTMEN JWT TESTER CLIENT ---")

	for {
		fmt.Println("\n--------------------------------")
		if authToken == "" {
			fmt.Println("1. Register")
			fmt.Println("2. Login")
		} else {
			fmt.Printf("Logged in as ID: %d | Role: %s\n", currentUserID, userRole)
			fmt.Println("3. List Channels")
			fmt.Println("4. Subscribe to Channel")
			fmt.Println("5. Update Phone (WhatsApp)")
			fmt.Println("6. View My Notifications")
			fmt.Println("7. Create Report (User)")
			if userRole == "moderator" {
				fmt.Println("8. [MOD] List Pending Reports")
				fmt.Println("9. [MOD] Approve Report")
			}
			fmt.Println("0. Logout")
		}
		fmt.Println("w. Send Debug Webhook")
		fmt.Println("q. Quit")
		fmt.Print("\n>> ")

		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1": auth("register")
		case "2": auth("login")
		case "3": getRequest("/channels")
		case "4": subscribe()
		case "5": setupPhone()
		case "6": getRequest("/notifications")
		case "7": createReport()
		case "8": getRequest("/moderation/pending")
		case "9": approveReport()
		case "0":
			authToken = ""
			currentUserID = 0
			userRole = ""
			fmt.Println("Logged out.")
		case "w": sendWebhook()
		case "q": return
		default: fmt.Println("Invalid input.")
		}
	}
}

// Универсальный POST запрос с JWT
func securePost(path string, data interface{}) ([]byte, int) {
	jsonData, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", baseURL+path, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Network error:", err)
		return nil, 500
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

// Универсальный GET запрос с JWT
func getRequest(path string) {
	req, _ := http.NewRequest("GET", baseURL+path, nil)
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("HTTP Error:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[%d] Response: %s\n", resp.StatusCode, string(body))
}

func auth(mode string) {
	fmt.Print("Username: ")
	scanner.Scan()
	user := strings.TrimSpace(scanner.Text())
	fmt.Print("Password: ")
	scanner.Scan()
	pass := strings.TrimSpace(scanner.Text())

	body, code := securePost("/"+mode, map[string]string{
		"username": user,
		"password": pass,
	})

	fmt.Printf("[%d] Server: %s\n", code, string(body))

	if code == 200 || code == 201 {
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if token, ok := result["token"].(string); ok {
			authToken = token
			currentUserID = uint(result["user_id"].(float64))
			userRole = result["role"].(string)
			fmt.Println("JWT Token saved successfully.")
		}
	}
}

func subscribe() {
	fmt.Print("Channel ID: ")
	scanner.Scan()
	id, _ := strconv.Atoi(scanner.Text())
	fmt.Print("Enable WhatsApp? (y/n): ")
	scanner.Scan()
	wa := strings.ToLower(scanner.Text()) == "y"

	body, code := securePost("/subscribe", map[string]interface{}{
		"channel_id":    uint(id),
		"send_whatsapp": wa,
	})
	fmt.Printf("[%d] Result: %s\n", code, string(body))
}

func setupPhone() {
	fmt.Print("Phone (digits): ")
	scanner.Scan()
	phone := strings.TrimSpace(scanner.Text())
	body, code := securePost("/profile/phone", map[string]string{"phone": phone})
	fmt.Printf("[%d] Result: %s\n", code, string(body))
}

func createReport() {
	fmt.Print("Channel ID: ")
	scanner.Scan()
	id, _ := strconv.Atoi(scanner.Text())
	fmt.Print("Title: ")
	scanner.Scan()
	title := scanner.Text()
	fmt.Print("Content: ")
	scanner.Scan()
	content := scanner.Text()

	body, code := securePost("/reports", map[string]interface{}{
		"channel_id": uint(id),
		"title":      title,
		"content":    content,
	})
	fmt.Printf("[%d] Result: %s\n", code, string(body))
}

func approveReport() {
	fmt.Print("Report ID to approve: ")
	scanner.Scan()
	id := strings.TrimSpace(scanner.Text())
	body, code := securePost("/moderation/approve/"+id, nil)
	fmt.Printf("[%d] Result: %s\n", code, string(body))
}

func sendWebhook() {
	fmt.Print("Type (Slug): ")
	scanner.Scan()
	t := scanner.Text()
	fmt.Print("Title: ")
	scanner.Scan()
	title := scanner.Text()
	fmt.Print("Content: ")
	scanner.Scan()
	content := scanner.Text()

	body, code := securePost("/webhook/send", map[string]string{
		"type": t, "title": title, "content": content,
	})
	fmt.Printf("[%d] Result: %s\n", code, string(body))
}
