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

func main() {
	fmt.Print("Enter server IP (0 for localhost:8080): ")
	scanner.Scan()
	inputIP := strings.TrimSpace(scanner.Text())
	if inputIP == "0" || inputIP == "" {
		baseURL = "http://localhost:8080/api"
	} else {
		baseURL = "http://" + inputIP + ":8080/api"
	}

	fmt.Println("\n--- ALERTMEN TESTER CLIENT ---")

	for {
		fmt.Println("\n--------------------------------")
		if currentUserID == 0 {
			fmt.Println("1. Register")
			fmt.Println("2. Login")
		} else {
			fmt.Printf("Logged in as User ID: %d\n", currentUserID)
			fmt.Println("3. Show Categories")
			fmt.Println("4. Subscribe to Category")
			fmt.Println("5. Bind Phone (WhatsApp)")
			fmt.Println("6. View My Notifications")
			fmt.Println("0. Logout")
		}
		fmt.Println("9. Send Webhook Event")
		fmt.Println("q. Quit")
		fmt.Print("\n>> ")

		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1": auth("register")
		case "2": auth("login")
		case "3": getCategories()
		case "4": subscribe()
		case "5": setupPhone()
		case "6": getNotifications()
		case "0":
			currentUserID = 0
			fmt.Println("Session cleared.")
		case "9": sendWebhook()
		case "q":
			return
		default:
			fmt.Println("Invalid input.")
		}
	}
}

func postRequest(path string, data interface{}) ([]byte, int) {
	jsonData, _ := json.Marshal(data)
	resp, err := http.Post(baseURL+path, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Network error: %v\n", err)
		return nil, 500
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

func auth(mode string) {
    // Force a clear read if there's a leftover newline
    fmt.Print("Enter Username: ")
    scanner.Scan() 
    user := strings.TrimSpace(scanner.Text())
    
    fmt.Print("Enter Password: ")
    scanner.Scan()
    pass := strings.TrimSpace(scanner.Text())

    // DEBUG: Add this line to see what the tester is actually sending
    fmt.Printf("Sending: %s with pass: %s\n", user, pass) 

    if user == "" || pass == "" {
        fmt.Println("Error: Empty fields not allowed.")
        return
    }

	body, code := postRequest("/"+mode, map[string]string{
		"username": user,
		"password": pass,
	})

	fmt.Printf("[%d] Server response: %s\n", code, string(body))
	
	if code == 200 || code == 201 {
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if idFloat, ok := result["user_id"].(float64); ok {
				currentUserID = uint(idFloat)
				fmt.Printf("Success! ID %d is now active.\n", currentUserID)
			}
		}
	}
}

func getCategories() {
	resp, err := http.Get(baseURL + "/categories")
	if err != nil {
		fmt.Println("HTTP Error:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Available Categories:", string(body))
}

func subscribe() {
	if currentUserID == 0 { return }
	
	fmt.Print("Category ID: ")
	scanner.Scan()
	catID, _ := strconv.Atoi(strings.TrimSpace(scanner.Text()))

	fmt.Print("Enable WhatsApp? (y/n): ")
	scanner.Scan()
	sendWA := strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"

	body, code := postRequest("/subscribe", map[string]interface{}{
		"user_id":       currentUserID,
		"category_id":   uint(catID),
		"send_whatsapp": sendWA,
	})
	fmt.Printf("[%d] Subscription result: %s\n", code, string(body))
}

func setupPhone() {
	if currentUserID == 0 { return }

	fmt.Print("Enter phone number (only digits, e.g. 77071234567): ")
	scanner.Scan()
	phone := strings.TrimSpace(scanner.Text())

	body, code := postRequest("/profile/phone", map[string]interface{}{
		"user_id": currentUserID,
		"phone":   phone,
	})
	fmt.Printf("[%d] Phone update result: %s\n", code, string(body))
}

func getNotifications() {
	if currentUserID == 0 { return }

	url := fmt.Sprintf("%s/notifications/%d", baseURL, currentUserID)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("HTTP Error:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("History:", string(body))
}

func sendWebhook() {
	fmt.Print("Event type (incident/weather/whatsapp): ")
	scanner.Scan()
	t := strings.TrimSpace(scanner.Text())
	
	fmt.Print("Title: ")
	scanner.Scan()
	title := strings.TrimSpace(scanner.Text())
	
	fmt.Print("Message content: ")
	scanner.Scan()
	content := strings.TrimSpace(scanner.Text())

	body, code := postRequest("/webhook/send", map[string]string{
		"type":    t,
		"title":   title,
		"content": content,
	})
	fmt.Printf("[%d] Webhook result: %s\n", code, string(body))
}
