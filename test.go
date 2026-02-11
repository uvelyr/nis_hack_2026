package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const baseURL = "https://72b0-31-169-21-231.ngrok-free.app/api"

var scanner = bufio.NewScanner(os.Stdin)
var currentUserID uint = 0

func main() {
	fmt.Println("=== Тестовый клиент системы уведомлений ===")

	for {
		fmt.Println("\nВыберите действие:")
		if currentUserID == 0 {
			fmt.Println("1. Регистрация")
			fmt.Println("2. Вход")
		} else {
			fmt.Printf("--- Вы вошли как ID: %d ---\n", currentUserID)
			fmt.Println("3. Просмотреть категории (Блок А)")
			fmt.Println("4. Подписаться на категорию")
			fmt.Println("5. Настроить WhatsApp (QR + Фильтр)")
			fmt.Println("6. Моя лента уведомлений")
			fmt.Println("7. Удалить уведомление")
			fmt.Println("0. Выйти из аккаунта")
		}
		fmt.Println("9. Имитировать внешнее событие (Webhook)")
		fmt.Println("q. Выход из программы")

		fmt.Print(">> ")
		scanner.Scan()
		choice := scanner.Text()

		switch choice {
		case "1":
			auth("register")
		case "2":
			auth("login")
		case "3":
			getCategories()
		case "4":
			subscribe()
		case "5":
			setupWA()
		case "6":
			getNotifications()
		case "7":
			deleteNotification()
		case "0":
			currentUserID = 0
			fmt.Println("Вы вышли.")
		case "9":
			sendWebhook()
		case "q":
			return
		default:
			fmt.Println("Неверный выбор.")
		}
	}
}

// Вспомогательная функция для POST запросов
func postRequest(path string, data interface{}) ([]byte, int) {
	jsonData, _ := json.Marshal(data)
	resp, err := http.Post(baseURL+path, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Ошибка соединения:", err)
		return nil, 500
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

func auth(mode string) {
	fmt.Print("Логин: ")
	scanner.Scan()
	user := scanner.Text()
	fmt.Print("Пароль: ")
	scanner.Scan()
	pass := scanner.Text()

	body, code := postRequest("/"+mode, map[string]string{
		"username": user,
		"password": pass,
	})

	fmt.Println("Ответ сервера:", string(body))
	if code == 200 || code == 201 {
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if id, ok := result["user_id"].(float64); ok {
			currentUserID = uint(id)
		}
	}
}

func getCategories() {
	resp, _ := http.Get(baseURL + "/categories")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Доступные категории:", string(body))
}

func subscribe() {
	fmt.Print("Введите ID категории для подписки: ")
	scanner.Scan()
	var catID uint
	fmt.Sscanf(scanner.Text(), "%d", &catID)

	body, _ := postRequest("/subscribe", map[string]uint{
		"user_id":     currentUserID,
		"category_id": catID,
	})
	fmt.Println("Результат:", string(body))
}

func setupWA() {
	fmt.Print("Введите фильтр текста (например, 'work'): ")
	scanner.Scan()
	filter := strings.ReplaceAll(scanner.Text(), " ", "%20")
	
	url := fmt.Sprintf("%s/whatsapp/setup?user_id=%d&filter=%s", baseURL, currentUserID, filter)
	resp, _ := http.Get(url)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Данные WhatsApp:", string(body))
}

func getNotifications() {
	url := fmt.Sprintf("%s/notifications/%d", baseURL, currentUserID)
	resp, _ := http.Get(url)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Ваши уведомления:", string(body))
}

func deleteNotification() {
	fmt.Print("ID уведомления для удаления: ")
	scanner.Scan()
	id := scanner.Text()
	
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/notifications/"+id, nil)
	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()
	fmt.Println("Удалено.")
}

func sendWebhook() {
	fmt.Println("Выберите тип (incident, weather, whatsapp):")
	scanner.Scan()
	t := scanner.Text()
	fmt.Print("Заголовок: ")
	scanner.Scan()
	title := scanner.Text()
	fmt.Print("Текст сообщения: ")
	scanner.Scan()
	content := scanner.Text()

	body, _ := postRequest("/webhook/send", map[string]string{
		"type":    t,
		"title":   title,
		"content": content,
	})
	fmt.Println("Результат рассылки:", string(body))
}
