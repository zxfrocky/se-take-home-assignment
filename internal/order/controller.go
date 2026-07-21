package order

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

var ErrNoBots = errors.New("no bots available")

type readyBot struct {
	botID   int
	orderCh chan *Order
}

type Bot struct {
	id      int
	stopCh  chan struct{}
	doneCh  chan struct{}
	orderCh chan *Order
	current *Order
	active  bool
}

type Controller struct {
	mu                 sync.Mutex
	logger             *EventLogger
	processingTime     time.Duration
	nextOrderID        int
	nextBotID          int
	pendingVIP         *list.List
	pendingNormal      *list.List
	vipCompleted       int
	normalCompleted    int
	bots               map[int]*Bot
	botOrder           []int
	idleBots           []readyBot
	readyCh            chan readyBot
	shutdownCh         chan struct{}
	shutdownOnce       sync.Once
	dispatchLoopDoneCh chan struct{}
}

func NewController(logger *EventLogger, processingTime time.Duration) *Controller {
	if processingTime <= 0 {
		processingTime = 10 * time.Second
	}
	c := &Controller{
		logger:             logger,
		processingTime:     processingTime,
		nextOrderID:        1001,
		nextBotID:          1,
		pendingVIP:         list.New(),
		pendingNormal:      list.New(),
		bots:               make(map[int]*Bot),
		readyCh:            make(chan readyBot),
		shutdownCh:         make(chan struct{}),
		dispatchLoopDoneCh: make(chan struct{}),
	}
	logger.Header()
	logger.Event("System initialized with 0 bots")
	go c.dispatchLoop()
	return c
}

func (c *Controller) CreateOrder(orderType OrderType) Order {
	if orderType != VIPOrder {
		orderType = NormalOrder
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	order := &Order{ID: c.nextOrderID, Type: orderType, Status: PendingStatus, CreatedAt: time.Now()}
	c.nextOrderID++
	c.enqueueLocked(order)
	c.logger.Event("Created %s Order #%d - Status: %s", order.Type, order.ID, order.Status)
	c.assignLocked()
	snapshot := *order
	return snapshot
}

func (c *Controller) AddBot() BotSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	bot := &Bot{id: c.nextBotID, stopCh: make(chan struct{}), doneCh: make(chan struct{}), orderCh: make(chan *Order, 1), active: true}
	c.nextBotID++
	c.bots[bot.id] = bot
	c.botOrder = append(c.botOrder, bot.id)
	c.logger.Event("Bot #%d created - Status: %s", bot.id, ActiveBot)
	go c.runBot(bot)
	snapshot := BotSnapshot{ID: bot.id, Status: ActiveBot}
	return snapshot
}

func (c *Controller) RemoveNewestBot() (BotSnapshot, error) {
	c.mu.Lock()

	if len(c.botOrder) == 0 {
		c.mu.Unlock()
		return BotSnapshot{}, ErrNoBots
	}
	id := c.botOrder[len(c.botOrder)-1]
	c.botOrder = c.botOrder[:len(c.botOrder)-1]
	bot := c.bots[id]
	delete(c.bots, id)
	bot.active = false
	filtered := c.idleBots[:0]
	for _, rb := range c.idleBots {
		if rb.botID != id {
			filtered = append(filtered, rb)
		}
	}
	c.idleBots = filtered
	processingID := 0
	if bot.current != nil {
		processingID = bot.current.ID
	}
	close(bot.stopCh)
	c.mu.Unlock()
	<-bot.doneCh
	return BotSnapshot{ID: id, Status: IdleBot, ProcessingID: processingID}, nil
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := Snapshot{TotalOrders: c.nextOrderID - 1001}
	for e := c.pendingVIP.Front(); e != nil; e = e.Next() {
		s.Pending = append(s.Pending, *e.Value.(*Order))
	}
	for e := c.pendingNormal.Front(); e != nil; e = e.Next() {
		s.Pending = append(s.Pending, *e.Value.(*Order))
	}
	s.VIPCompleted = c.vipCompleted
	s.NormCompleted = c.normalCompleted
	for _, id := range c.botOrder {
		if bot, ok := c.bots[id]; ok {
			bs := BotSnapshot{ID: id, Status: IdleBot}
			if bot.current != nil {
				bs.Status = ActiveBot
				bs.ProcessingID = bot.current.ID
				bs.ProcessingVIP = bot.current.Type == VIPOrder
				s.Processing = append(s.Processing, *bot.current)
			}
			s.Bots = append(s.Bots, bs)
		}
	}
	return s
}

func (c *Controller) WriteFinalStatus() {
	s := c.Snapshot()
	completed := s.VIPCompleted + s.NormCompleted
	c.logger.BlankLine()
	c.logger.Line("Final Status:")
	c.logger.Line("- Total Orders Processed: %d (%d VIP, %d Normal)", completed, s.VIPCompleted, s.NormCompleted)
	c.logger.Line("- Orders Completed: %d", completed)
	c.logger.Line("- Active Bots: %d", len(s.Bots))
	c.logger.Line("- Pending Orders: %d", len(s.Pending))
}

func (c *Controller) Shutdown() {
	c.shutdownOnce.Do(func() {
		close(c.shutdownCh)
		c.mu.Lock()
		bots := make([]*Bot, 0, len(c.botOrder))
		for _, id := range c.botOrder {
			if bot, ok := c.bots[id]; ok && bot.active {
				bot.active = false
				close(bot.stopCh)
				bots = append(bots, bot)
			}
		}
		c.mu.Unlock()
		<-c.dispatchLoopDoneCh
		for _, bot := range bots {
			<-bot.doneCh
		}
	})
}

func (c *Controller) dispatchLoop() {
	defer close(c.dispatchLoopDoneCh)
	for {
		select {
		case rb := <-c.readyCh:
			func() {
				c.mu.Lock()
				defer c.mu.Unlock()

				bot, ok := c.bots[rb.botID]
				if ok && bot.active {
					c.idleBots = append(c.idleBots, rb)
					if c.pendingVIP.Len() == 0 && c.pendingNormal.Len() == 0 {
						c.logger.Event("Bot #%d is now IDLE - No pending orders", rb.botID)
					}
					c.assignLocked()
				}
			}()
		case <-c.shutdownCh:
			return
		}
	}
}

func (c *Controller) runBot(bot *Bot) {
	defer close(bot.doneCh)
	for {
		select {
		case c.readyCh <- readyBot{botID: bot.id, orderCh: bot.orderCh}:
		case <-bot.stopCh:
			c.logger.Event("Bot #%d destroyed while IDLE", bot.id)
			return
		case <-c.shutdownCh:
			return
		}

		select {
		case order := <-bot.orderCh:
			c.logger.Event("Bot #%d picked up %s Order #%d - Status: %s", bot.id, order.Type, order.ID, order.Status)
			timer := time.NewTimer(c.processingTime)
			select {
			case <-timer.C:
				c.completeOrder(bot, order)
			case <-bot.stopCh:
				timer.Stop()
				c.returnOrder(bot, order)
				return
			case <-c.shutdownCh:
				timer.Stop()
				c.returnOrder(bot, order)
				return
			}
		case <-bot.stopCh:
			c.drainOrderOrIdle(bot)
			return
		case <-c.shutdownCh:
			c.drainOrderOrIdle(bot)
			return
		}
	}
}

func (c *Controller) drainOrderOrIdle(bot *Bot) {
	select {
	case order := <-bot.orderCh:
		c.returnOrder(bot, order)
	default:
		c.logger.Event("Bot #%d destroyed while IDLE", bot.id)
	}
}

func (c *Controller) completeOrder(bot *Bot, order *Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if bot.active && bot.current != nil && bot.current.ID == order.ID {
		order.Status = CompleteStatus
		order.DoneAt = time.Now()
		bot.current = nil
		if order.Type == VIPOrder {
			c.vipCompleted++
		} else {
			c.normalCompleted++
		}
		c.logger.Event("Bot #%d completed %s Order #%d - Status: %s (Processing time: 10s)", bot.id, order.Type, order.ID, order.Status)
	}
}

func (c *Controller) returnOrder(bot *Bot, order *Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	order.Status = PendingStatus
	order.StartedAt = time.Time{}
	bot.current = nil
	c.reEnqueueLocked(order)
	c.logger.Event("Bot #%d destroyed while processing %s Order #%d - Status: PENDING", bot.id, order.Type, order.ID)
	c.assignLocked()
}

func (c *Controller) enqueueLocked(order *Order) {
	order.Status = PendingStatus
	if order.Type == VIPOrder {
		c.pendingVIP.PushBack(order)
		return
	}
	c.pendingNormal.PushBack(order)
}

// reEnqueueLocked 把被中断的订单退回 pending 队首，保留它的"创建顺序优先级"。
// 与 enqueueLocked（新单入队尾）相对。
func (c *Controller) reEnqueueLocked(order *Order) {
	order.Status = PendingStatus
	if order.Type == VIPOrder {
		c.pendingVIP.PushFront(order)
		return
	}
	c.pendingNormal.PushFront(order)
}

func (c *Controller) assignLocked() {
	for len(c.idleBots) > 0 && (c.pendingVIP.Len() > 0 || c.pendingNormal.Len() > 0) {
		rb := c.idleBots[0]
		c.idleBots = c.idleBots[1:]
		bot, ok := c.bots[rb.botID]
		if !ok || !bot.active {
			continue
		}
		var order *Order
		if c.pendingVIP.Len() > 0 {
			order = c.pendingVIP.Remove(c.pendingVIP.Front()).(*Order)
		} else {
			order = c.pendingNormal.Remove(c.pendingNormal.Front()).(*Order)
		}
		order.Status = ProcessingStatus
		order.StartedAt = time.Now()
		bot.current = order
		select {
		case rb.orderCh <- order:
		case <-bot.stopCh:
			bot.current = nil
			order.Status = PendingStatus
			c.reEnqueueLocked(order)
		default:
			bot.current = nil
			order.Status = PendingStatus
			c.reEnqueueLocked(order)
		}
	}
}
