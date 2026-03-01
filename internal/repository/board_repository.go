package repository

import (
	"errors"
	"sort"
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

var ErrBoardNotFound = errors.New("board not found")

type BoardRepository interface {
	CreateBoard(board *domain.Board) error
	ListBoards() ([]domain.Board, error)
	CreateMessage(msg *domain.Message) error
	ListMessagesByBoard(boardID int64) ([]domain.Message, error)
}

type InMemoryBoardRepository struct {
	mu          sync.RWMutex
	nextBoardID int64
	nextMsgID   int64
	boards      map[int64]*domain.Board
	boardOrder  []int64
	messages    map[int64]*domain.Message
	byBoard     map[int64][]int64
}

func NewInMemoryBoardRepository() *InMemoryBoardRepository {
	r := &InMemoryBoardRepository{
		nextBoardID: 1,
		nextMsgID:   1,
		boards:      map[int64]*domain.Board{},
		messages:    map[int64]*domain.Message{},
		byBoard:     map[int64][]int64{},
	}
	_ = r.CreateBoard(&domain.Board{Name: "General", Description: "General discussion", CreatedBy: 1})
	_ = r.CreateBoard(&domain.Board{Name: "Announcements", Description: "WolfBBS news", CreatedBy: 1})
	return r
}

func (r *InMemoryBoardRepository) CreateBoard(board *domain.Board) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	board.ID = r.nextBoardID
	r.nextBoardID++
	board.CreatedAt = now
	copy := *board
	r.boards[copy.ID] = &copy
	r.boardOrder = append(r.boardOrder, copy.ID)
	return nil
}

func (r *InMemoryBoardRepository) ListBoards() ([]domain.Board, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Board, 0, len(r.boardOrder))
	for _, id := range r.boardOrder {
		out = append(out, *r.boards[id])
	}
	return out, nil
}

func (r *InMemoryBoardRepository) CreateMessage(msg *domain.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.boards[msg.BoardID]; !ok {
		return ErrBoardNotFound
	}
	now := time.Now().UTC()
	msg.ID = r.nextMsgID
	r.nextMsgID++
	msg.CreatedAt = now
	copy := *msg
	r.messages[copy.ID] = &copy
	r.byBoard[msg.BoardID] = append(r.byBoard[msg.BoardID], copy.ID)
	return nil
}

func (r *InMemoryBoardRepository) ListMessagesByBoard(boardID int64) ([]domain.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.boards[boardID]; !ok {
		return nil, ErrBoardNotFound
	}
	ids := append([]int64(nil), r.byBoard[boardID]...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]domain.Message, 0, len(ids))
	for _, id := range ids {
		out = append(out, *r.messages[id])
	}
	return out, nil
}
