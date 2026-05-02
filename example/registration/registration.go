// Пакет registration — демонстрационный пример для testgen.
//
// Содержит функцию с несколькими guards по полям структуры.
// Хорошо показывает, как testgen генерирует edge-сценарии:
// analyzer обнаруживает проверки email == "", name == "", age < 18
// и строит соответствующие Guards.EmptyCheckedParams.
//
// Запуск:
//
//	go run ./cmd/testgen -validate ./example/registration
package registration

import (
	"errors"
	"strings"
	"time"
)

// Address — адресные данные пользователя.
type Address struct {
	City   string
	Street string
	House  string
}

// RegisterRequest — запрос на регистрацию нового пользователя.
type RegisterRequest struct {
	Email     string
	Name      string
	Age       int
	Phone     string
	Address   Address
	CreatedAt time.Time
}

// ValidateRegisterRequest проверяет корректность запроса на регистрацию.
// Возвращает первую найденную ошибку валидации или nil.
//
// Проверки (в порядке приоритета):
//  1. email не может быть пустым
//  2. email должен содержать "@"
//  3. name не может быть пустым
//  4. age должен быть >= 18
//  5. city адреса не может быть пустым
func ValidateRegisterRequest(req RegisterRequest) error {
	if req.Email == "" {
		return errors.New("email обязателен")
	}
	if !strings.Contains(req.Email, "@") {
		return errors.New("email должен содержать @")
	}
	if req.Name == "" {
		return errors.New("имя обязательно")
	}
	if req.Age < 18 {
		return errors.New("возраст должен быть не менее 18 лет")
	}
	if req.Address.City == "" {
		return errors.New("город обязателен")
	}
	return nil
}
