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
)

var (
	baseURL     = "http://localhost:8080/api"
	scanner     = bufio.NewScanner(os.Stdin)
	authToken   string
	isModerator bool
)

func main() {
	for {
		fmt.Println("\n--- ALERTMEN TESTER ---")
		if authToken == "" {
			fmt.Println("1. Register | 2. Login | q. Quit")
		} else {
			fmt.Printf("Logged in (Moderator: %v)\n", isModerator)
			fmt.Println("3. List Channels | 4. Subscribe | 5. Send Report")
			fmt.Println("6. Notifications History")
			if isModerator {
				fmt.Println("8. [MOD] View Inbox | 9. [MOD] Approve | 10. [MOD] Reject")
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
		case "5": createReport()
		case "6": getRequest("/notifications")
		case "8": if isModerator { viewInbox() }
		case "9": if isModerator { moderate("approve") }
		case "10": if isModerator { moderate("reject") }
		case "0": authToken = ""; isModerator = false
		case "q": return
		}
	}
}

func auth(mode string) {
	fmt.Print("Username: "); scanner.Scan(); u := scanner.Text()
	fmt.Print("Password: "); scanner.Scan(); p := scanner.Text()
	data, code := postRequest("/"+mode, map[string]string{"username": u, "password": p})
	fmt.Printf("[%d] Response: %s\n", code, string(data))
	if code == 200 {
		var res map[string]interface{}
		json.Unmarshal(data, &res)
		authToken = res["token"].(string)
		isModerator = res["is_moderator"].(bool)
	}
}

func viewInbox() {
	data, _ := getRequestData("/moderation/inbox")
	var reports []map[string]interface{}
	json.Unmarshal(data, &reports)
	fmt.Println("\n--- PENDING REPORTS ---")
	for _, r := range reports {
		fmt.Printf("ID: %v | Title: %s | Content: %s\n", r["id"], r["title"], r["content"])
	}
}

func moderate(action string) {
	fmt.Print("Report ID: "); scanner.Scan(); id := scanner.Text()
	_, code := postRequest("/moderation/"+action+"/"+id, nil)
	fmt.Printf("Result: %d\n", code)
}

// --- HELPERS ---

func postRequest(path string, payload interface{}) ([]byte, int) {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", baseURL+path, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" { req.Header.Set("Authorization", "Bearer "+authToken) }
	resp, err := (&http.Client{}).Do(req)
	if err != nil { return nil, 500 }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

func getRequestData(path string) ([]byte, int) {
	req, _ := http.NewRequest("GET", baseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+authToken)
	resp, _ := (&http.Client{}).Do(req)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

func getRequest(path string) {
	d, c := getRequestData(path)
	fmt.Printf("[%d] %s\n", c, string(d))
}

func subscribe() {
	fmt.Print("CH ID: "); scanner.Scan(); id, _ := strconv.Atoi(scanner.Text())
	postRequest("/subscribe", map[string]interface{}{"channel_id": id, "send_whatsapp": true})
}

func createReport() {
	fmt.Print("CH ID: "); scanner.Scan(); id, _ := strconv.Atoi(scanner.Text())
	fmt.Print("Title: "); scanner.Scan(); t := scanner.Text()
	fmt.Print("Content: "); scanner.Scan(); c := scanner.Text()
	postRequest("/reports", map[string]interface{}{"channel_id": id, "title": t, "content": c})
}
