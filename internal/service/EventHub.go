package service

import (
	"sync"
	"time"
)

// StreamEvent — лёгкий сигнал для realtime через SSE. Тело сущности НЕ передаём;
// фронт по сигналу инвалидирует нужные кэши и дозапрашивает REST (как в myacc).
type StreamEvent struct {
	Type       string `json:"type"`
	WorkItemID string `json:"work_item_id,omitempty"`
	Ts         int64  `json:"ts"`
}

// EventHub — in-memory брокер fan-out для SSE. Простая широковещательная шина:
// событие рассылается всем подключённым клиентам, каждый сам решает что инвалидировать.
// Для нескольких реплик API нужно заменить на Redis pub/sub (интерфейс сохранится).
type EventHub struct {
	mu   sync.RWMutex
	subs map[chan StreamEvent]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{subs: make(map[chan StreamEvent]struct{})}
}

// Subscribe возвращает канал событий и функцию отписки.
func (h *EventHub) Subscribe() (<-chan StreamEvent, func()) {
	ch := make(chan StreamEvent, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	cleanup := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, cleanup
}

// Publish неблокирующе рассылает событие подписчикам (медленных пропускаем).
func (h *EventHub) Publish(ev StreamEvent) {
	if ev.Ts == 0 {
		ev.Ts = time.Now().Unix()
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// клиент не успевает — пропускаем сигнал (он дозапросит при следующем)
		}
	}
}

// GlobalEventHub — опциональный глобальный хаб, в который пишет conveyorService.createEvent.
// main.go выставляет его; в тестах nil → публикация no-op.
var GlobalEventHub *EventHub

func publishGlobal(ev StreamEvent) {
	if GlobalEventHub != nil {
		GlobalEventHub.Publish(ev)
	}
}
