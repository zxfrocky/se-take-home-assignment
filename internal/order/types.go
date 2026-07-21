package order

import "time"

type OrderType string

const (
	NormalOrder OrderType = "Normal"
	VIPOrder    OrderType = "VIP"
)

type OrderStatus string

const (
	PendingStatus    OrderStatus = "PENDING"
	ProcessingStatus OrderStatus = "PROCESSING"
	CompleteStatus   OrderStatus = "COMPLETE"
)

type Order struct {
	ID        int         `json:"id"`
	Type      OrderType   `json:"type"`
	Status    OrderStatus `json:"status"`
	CreatedAt time.Time   `json:"createdAt"`
	StartedAt time.Time   `json:"startedAt,omitempty"`
	DoneAt    time.Time   `json:"doneAt,omitempty"`
}

type BotStatus string

const (
	ActiveBot BotStatus = "ACTIVE"
	IdleBot   BotStatus = "IDLE"
)

type BotSnapshot struct {
	ID            int       `json:"id"`
	Status        BotStatus `json:"status"`
	ProcessingID  int       `json:"processingId,omitempty"`
	ProcessingVIP bool      `json:"processingVip,omitempty"`
}

type Snapshot struct {
	Pending       []Order       `json:"pending"`
	Processing    []Order       `json:"processing"`
	Bots          []BotSnapshot `json:"bots"`
	TotalOrders   int           `json:"totalOrders"`
	VIPCompleted  int           `json:"vipCompleted"`
	NormCompleted int           `json:"normalCompleted"`
}
