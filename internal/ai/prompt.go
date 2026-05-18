package ai

import (
	"fmt"
	"strings"

	"github.com/palemoky/fight-the-landlord/internal/game/card"
)

const systemPrompt = `你是专业斗地主AI，目标是赢得游戏。
【规则】3人：地主(20张)vs农民×2(17张)。3<4<5<6<7<8<9<T<J<Q<K<A<2<B<R(B=小王,R=大王；T=10)
【牌型】（必须严格符合，否则非法）
- 单/对/三：1/2/3张同点
- 三带一/三带对：3张同点+1张单/1对
- 顺子：≥5张连续单点，必须严格连续(如3 4 5 6 7)，不含2/小王/大王
- 连对：≥3组连续对子(如33 44 55)，不含2/王
- 飞机：≥2组连续三张，可各带1单或1对，不含2/王
- 炸弹：4张同点；王炸：小王+大王(最大)
【压牌】同牌型且张数相同才能比大小；炸弹压非炸弹；王炸压一切
【阵营策略】
- 地主：消耗农民的牌，控制节奏，最后清空手牌取胜
- 农民：两人配合击败地主，伺机让队友控场，必要时主动接牌配合
【出牌原则】
- 控场/开局：出最小的散牌或长顺子/连对消耗手牌，绝不用大牌(A/2)、炸弹或王炸开局
- 跟牌：用刚好能压过且最小的牌应对，省下大牌
- 炸弹/王炸只在对手剩≤2张或必须阻止对手出完时才用，否则一律保留
- 只能使用当前手牌中真实拥有的点数，张数必须与牌型匹配
【输出格式】只输出符合【牌型】之一的牌面，空格分隔如"3 3 K"，pass=不出，10写成T，禁止任何解释、标点或思考过程`

func buildSystemPrompt() string {
	return systemPrompt
}

func buildPlayPrompt(gctx GameContext) string {
	var sb strings.Builder

	role := "农民"
	if gctx.IsLandlord {
		role = "地主"
	}

	var lastPlayedStr string
	if gctx.LastPlayed.IsEmpty() {
		lastPlayedStr = "新轮次"
	} else {
		lastPlayedStr = fmt.Sprintf("%s出:%s", gctx.LastPlayerName, cardsToStr(gctx.LastPlayed.Cards))
	}

	var actionStr string
	switch {
	case gctx.MustPlay:
		actionStr = "【必须出牌(新轮次/控场)，出最小的散牌或长连牌，禁止大牌/炸弹开局】"
	case gctx.CanBeat:
		actionStr = "【有牌可打，用刚好能压过的最小牌获得控场权】"
	default:
		actionStr = "【无牌可打，只能pass】"
	}

	fmt.Fprintf(&sb, "身份:%s 手牌(%d张):%s\n", role, len(gctx.Hand), cardsToStr(gctx.Hand))
	fmt.Fprintf(&sb, "上家:%s\n", lastPlayedStr)
	fmt.Fprintf(&sb, "左%d张 右%d张\n", gctx.PlayerCounts[0], gctx.PlayerCounts[1])

	if len(gctx.SeenCards) > 0 {
		seenParts := make([]string, 0, len(gctx.SeenCards))
		for rank, count := range gctx.SeenCards {
			if count > 0 {
				seenParts = append(seenParts, fmt.Sprintf("%s×%d", rank, count))
			}
		}
		if len(seenParts) > 0 {
			fmt.Fprintf(&sb, "记牌已出:%s\n", strings.Join(seenParts, " "))
		}
	}

	fmt.Fprintf(&sb, "%s 出什么?", actionStr)
	return sb.String()
}

func buildBidPrompt(hand []card.Card) string {
	return fmt.Sprintf("手牌(%d张):%s\n叫地主?(叫/不叫)", len(hand), cardsToStr(hand))
}

func cardsToStr(cards []card.Card) string {
	parts := make([]string, len(cards))
	for i, c := range cards {
		parts[i] = c.Rank.String()
	}
	return strings.Join(parts, " ")
}
