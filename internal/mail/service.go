package mail

import (
	"errors"
	"strings"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

type Service struct {
	mailRepo repository.MailRepository
	users    repository.UserRepository
}

func NewService(mailRepo repository.MailRepository, users repository.UserRepository) *Service {
	return &Service{mailRepo: mailRepo, users: users}
}

func (s *Service) Send(fromUserID int64, toHandle, subject, body string) error {
	toHandle = strings.TrimSpace(toHandle)
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if toHandle == "" || subject == "" || body == "" {
		return errors.New("to, subject, and body are required")
	}
	toUser, err := s.users.GetByHandle(toHandle)
	if err != nil {
		return errors.New("recipient not found")
	}
	return s.mailRepo.Send(&domain.PrivateMail{
		FromUserID: fromUserID,
		ToUserID:   toUser.ID,
		Subject:    subject,
		Body:       body,
	})
}

func (s *Service) Inbox(userID int64) ([]domain.PrivateMail, error) {
	return s.mailRepo.ListInbox(userID)
}
func (s *Service) Outbox(userID int64) ([]domain.PrivateMail, error) {
	return s.mailRepo.ListOutbox(userID)
}
