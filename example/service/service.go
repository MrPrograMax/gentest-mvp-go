// Пакет service — демонстрационный пример для minimock-интеграции.
//
// Содержит:
//   - доменную модель User
//   - интерфейс-зависимость UserRepository
//   - структуру Service с repo как поле-интерфейс
//   - метод GetUserName, который вызывает repo и содержит guards
//
// На этом примере testgen с флагом --mock=minimock:
//   - автоматически генерирует example/service/mock/user_repository_mock.go
//   - создаёт testgen_generated_test.go со scaffold minimock-инфраструктуры
//
// Запуск:
//
//	go run ./cmd/testgen --mock=minimock -validate ./example/service
package service

import (
	"context"
	"errors"
)

// User — простая доменная модель.
type User struct {
	ID   int64
	Name string
}

// UserRepository — интерфейс-зависимость, которую Service использует.
// В тестах вместо реальной реализации подставляется minimock-мок.
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (User, error)
}

// Service содержит интерфейсную зависимость UserRepository как поле.
// Именно поля-интерфейсы receiver-структуры являются мишенью для mockplan.
type Service struct {
	repo UserRepository
}

// NewService — стандартный конструктор для DI.
func NewService(repo UserRepository) *Service {
	return &Service{repo: repo}
}

// GetUserName возвращает имя пользователя по ID.
// Содержит guards (ctx == nil, id <= 0) и вызов repo.GetByID,
// чьи ошибки оборачиваются для удобства тестирования.
func (s *Service) GetUserName(ctx context.Context, id int64) (string, error) {
	if ctx == nil {
		return "", errors.New("nil context")
	}
	if id <= 0 {
		return "", errors.New("non-positive id")
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	return user.Name, nil
}
