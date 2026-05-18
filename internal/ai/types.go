package ai

import (
	"github.com/palemoky/fight-the-landlord/internal/game/card"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
	"github.com/palemoky/fight-the-landlord/internal/protocol"
)

// SessionInterface 避免 session↔ai 循环依赖
type SessionInterface interface {
	HandleBid(playerID string, bid bool) error
	HandlePlayCards(playerID string, cardInfos []protocol.CardInfo) error
	HandlePass(playerID string) error
}

// GameContext LLM 决策所需的游戏状态
type GameContext struct {
	BotID          string
	IsLandlord     bool
	Hand           []card.Card
	LastPlayed     rule.ParsedHand
	LastPlayerName string
	MustPlay       bool
	CanBeat        bool
	PlayerCounts   [2]int            // 其他两名玩家的剩余牌数（按座位顺序）
	SeenCards      map[card.Rank]int // 已出牌数量（各点数）
}
