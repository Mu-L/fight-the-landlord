package ai

import (
	"context"
	"fmt"
	"log"
	"maps"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/palemoky/fight-the-landlord/internal/game/card"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
	"github.com/palemoky/fight-the-landlord/internal/protocol"
	"github.com/palemoky/fight-the-landlord/internal/protocol/codec"
	"github.com/palemoky/fight-the-landlord/internal/protocol/convert"
)

var botNames = []string{
	"运筹帷幄", "算无遗策", "天下无双", "牌技精湛", "料事如神",
	"出奇制胜", "胸有成竹", "稳操胜券", "势如破竹", "百战百胜",
}

// AIBotClient 实现 types.ClientInterface 的 AI 机器人
type AIBotClient struct {
	id     string
	name   string
	engine *Engine

	roomMu sync.RWMutex
	room   string

	sessionMu sync.RWMutex
	session   SessionInterface

	closedMu sync.RWMutex
	closed   bool

	state botState
}

type botState struct {
	mu             sync.RWMutex
	hand           []card.Card
	isLandlord     bool
	cardCounts     map[string]int    // playerID → 剩余牌数
	playerNames    map[string]string // playerID → name
	orderedOthers  []string          // 除自己外按座位顺序的 playerID（2人）
	lastPlayed     rule.ParsedHand
	lastPlayerName string
	seenCards      map[card.Rank]int
}

// NewAIBotClient 创建 AI 机器人客户端
func NewAIBotClient(engine *Engine) *AIBotClient {
	name := fmt.Sprintf("🤖%s", botNames[rand.IntN(len(botNames))])
	return &AIBotClient{
		id:     uuid.New().String(),
		name:   name,
		engine: engine,
		state: botState{
			cardCounts:  make(map[string]int),
			playerNames: make(map[string]string),
			seenCards:   make(map[card.Rank]int),
		},
	}
}

// SetSession 在 GameSession 创建后注入（由 matcher 调用）
func (b *AIBotClient) SetSession(s SessionInterface) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	b.session = s
}

// --- types.ClientInterface 实现 ---

func (b *AIBotClient) GetID() string   { return b.id }
func (b *AIBotClient) GetName() string { return b.name }

func (b *AIBotClient) GetRoom() string {
	b.roomMu.RLock()
	defer b.roomMu.RUnlock()
	return b.room
}

func (b *AIBotClient) SetRoom(code string) {
	b.roomMu.Lock()
	defer b.roomMu.Unlock()
	b.room = code
}

func (b *AIBotClient) Close() {
	b.closedMu.Lock()
	defer b.closedMu.Unlock()
	b.closed = true
}

func (b *AIBotClient) IsBot() bool { return true }

func (b *AIBotClient) SendMessage(msg *protocol.Message) {
	b.closedMu.RLock()
	closed := b.closed
	b.closedMu.RUnlock()
	if closed {
		return
	}

	switch msg.Type {
	case protocol.MsgGameStart:
		b.handleGameStart(msg)
	case protocol.MsgDealCards:
		b.handleDealCards(msg)
	case protocol.MsgLandlord:
		b.handleLandlord(msg)
	case protocol.MsgCardPlayed:
		b.handleCardPlayed(msg)
	case protocol.MsgBidTurn:
		go b.handleBidTurn(msg)
	case protocol.MsgPlayTurn:
		go b.handlePlayTurn(msg)
	case protocol.MsgGameOver:
		b.engine.ClearHistory(b.id)
	}
}

// --- 消息处理 ---

func (b *AIBotClient) handleGameStart(msg *protocol.Message) {
	payload, err := codec.ParsePayload[protocol.GameStartPayload](msg)
	if err != nil {
		log.Printf("🤖 handleGameStart decode error: %v", err)
		return
	}

	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	b.state.playerNames = make(map[string]string)
	b.state.cardCounts = make(map[string]int)
	b.state.orderedOthers = make([]string, 0, 2)

	for _, p := range payload.Players {
		b.state.playerNames[p.ID] = p.Name
		b.state.cardCounts[p.ID] = 17
		if p.ID != b.id {
			b.state.orderedOthers = append(b.state.orderedOthers, p.ID)
		}
	}
	b.state.seenCards = make(map[card.Rank]int)
	b.state.lastPlayed = rule.ParsedHand{}
	b.state.lastPlayerName = ""
	b.state.isLandlord = false
}

func (b *AIBotClient) handleDealCards(msg *protocol.Message) {
	payload, err := codec.ParsePayload[protocol.DealCardsPayload](msg)
	if err != nil {
		log.Printf("🤖 handleDealCards decode error: %v", err)
		return
	}

	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.hand = convert.InfosToCards(payload.Cards)
	log.Printf("🤖 %s 收到手牌 %d 张", b.name, len(b.state.hand))
}

func (b *AIBotClient) handleLandlord(msg *protocol.Message) {
	payload, err := codec.ParsePayload[protocol.LandlordPayload](msg)
	if err != nil {
		log.Printf("🤖 handleLandlord decode error: %v", err)
		return
	}

	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	if payload.PlayerID == b.id {
		b.state.isLandlord = true
		// 地主手牌已通过 MsgDealCards 更新（session 会单独发送含底牌的手牌）
	}
	// 更新地主的牌数（+3 底牌）
	if _, ok := b.state.cardCounts[payload.PlayerID]; ok {
		b.state.cardCounts[payload.PlayerID] += 3
	}
}

func (b *AIBotClient) handleCardPlayed(msg *protocol.Message) {
	payload, err := codec.ParsePayload[protocol.CardPlayedPayload](msg)
	if err != nil {
		log.Printf("🤖 handleCardPlayed decode error: %v", err)
		return
	}

	played := convert.InfosToCards(payload.Cards)

	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	// 更新剩余牌数
	b.state.cardCounts[payload.PlayerID] = payload.CardsLeft

	// 如果是自己出的牌，从手牌中移除
	if payload.PlayerID == b.id {
		b.state.hand = removeCards(b.state.hand, played)
	}

	// 更新已出牌记录
	for _, c := range played {
		b.state.seenCards[c.Rank]++
	}

	// 更新上家出牌
	parsed, parseErr := rule.ParseHand(played)
	if parseErr == nil && parsed.Type != rule.Invalid {
		b.state.lastPlayed = parsed
		b.state.lastPlayerName = payload.PlayerName
	}
}

func (b *AIBotClient) handleBidTurn(msg *protocol.Message) {
	payload, err := codec.ParsePayload[protocol.BidTurnPayload](msg)
	if err != nil {
		log.Printf("🤖 handleBidTurn decode error: %v", err)
		return
	}
	if payload.PlayerID != b.id {
		return
	}

	time.Sleep(thinkDelay())

	b.state.mu.RLock()
	hand := make([]card.Card, len(b.state.hand))
	copy(hand, b.state.hand)
	b.state.mu.RUnlock()

	bid := b.engine.DecideBid(context.Background(), b.id, hand)

	b.sessionMu.RLock()
	sess := b.session
	b.sessionMu.RUnlock()

	if sess == nil {
		log.Printf("🤖 %s: session 未就绪，跳过叫地主", b.name)
		return
	}

	if err := sess.HandleBid(b.id, bid); err != nil {
		log.Printf("🤖 %s HandleBid 失败: %v", b.name, err)
	}
}

func (b *AIBotClient) handlePlayTurn(msg *protocol.Message) {
	payload, err := codec.ParsePayload[protocol.PlayTurnPayload](msg)
	if err != nil {
		log.Printf("🤖 handlePlayTurn decode error: %v", err)
		return
	}
	if payload.PlayerID != b.id {
		return
	}

	time.Sleep(thinkDelay())

	b.state.mu.RLock()
	gctx := b.buildGameContext(payload.MustPlay, payload.CanBeat)
	b.state.mu.RUnlock()

	b.sessionMu.RLock()
	sess := b.session
	b.sessionMu.RUnlock()

	if sess == nil {
		log.Printf("🤖 %s: session 未就绪，跳过出牌", b.name)
		return
	}

	cards := b.engine.DecidePlay(context.Background(), b.id, b.name, gctx)

	var playErr error
	if cards == nil {
		playErr = sess.HandlePass(b.id)
	} else {
		playErr = sess.HandlePlayCards(b.id, convert.CardsToInfos(cards))
	}

	if playErr != nil {
		log.Printf("🤖 %s 出牌失败: %v", b.name, playErr)
	}
}

// buildGameContext 构建 LLM 决策上下文（调用时需持有 state.mu.RLock）
func (b *AIBotClient) buildGameContext(mustPlay, canBeat bool) GameContext {
	hand := make([]card.Card, len(b.state.hand))
	copy(hand, b.state.hand)

	seenCards := make(map[card.Rank]int, len(b.state.seenCards))
	maps.Copy(seenCards, b.state.seenCards)

	var counts [2]int
	if len(b.state.orderedOthers) > 0 {
		counts[0] = b.state.cardCounts[b.state.orderedOthers[0]]
	}
	if len(b.state.orderedOthers) > 1 {
		counts[1] = b.state.cardCounts[b.state.orderedOthers[1]]
	}

	lastPlayed := b.state.lastPlayed
	lastPlayerName := b.state.lastPlayerName
	if mustPlay {
		lastPlayed = rule.ParsedHand{}
		lastPlayerName = ""
	}

	return GameContext{
		BotID:          b.id,
		IsLandlord:     b.state.isLandlord,
		Hand:           hand,
		LastPlayed:     lastPlayed,
		LastPlayerName: lastPlayerName,
		MustPlay:       mustPlay,
		CanBeat:        canBeat,
		PlayerCounts:   counts,
		SeenCards:      seenCards,
	}
}

// thinkDelay 模拟思考时间（300–900ms）
func thinkDelay() time.Duration {
	return time.Duration(300+rand.IntN(600)) * time.Millisecond
}

// removeCards 从 hand 中移除 played 中的牌（按 Rank+Suit 精确匹配）
func removeCards(hand, played []card.Card) []card.Card {
	type key struct {
		suit int
		rank card.Rank
	}
	toRemove := make(map[key]int)
	for _, c := range played {
		toRemove[key{int(c.Suit), c.Rank}]++
	}
	result := make([]card.Card, 0, len(hand)-len(played))
	for _, c := range hand {
		k := key{int(c.Suit), c.Rank}
		if toRemove[k] > 0 {
			toRemove[k]--
		} else {
			result = append(result, c)
		}
	}
	return result
}
