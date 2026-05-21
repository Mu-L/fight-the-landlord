package ai

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/joho/godotenv"

	"github.com/palemoky/fight-the-landlord/internal/config"
	"github.com/palemoky/fight-the-landlord/internal/game/card"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
)

// --- 测试辅助函数 ---

// cards 从空格分隔的牌面字符串构建 []card.Card（花色统一用黑桃，不影响规则判断）
func cards(notation string) []card.Card {
	tokens := strings.Fields(strings.ToUpper(notation))
	result := make([]card.Card, 0, len(tokens))
	for _, token := range tokens {
		var rank card.Rank
		if token == "10" {
			rank = card.Rank10
		} else {
			r, err := card.RankFromChar(rune(token[0]))
			if err != nil {
				panic(fmt.Sprintf("无效牌面记号: %q", token))
			}
			rank = r
		}
		result = append(result, card.Card{Rank: rank, Suit: card.Spade})
	}
	return result
}

// play 从牌面字符串构建 PlayRecord
func play(notation string, isLandlord bool) PlayRecord {
	c := cards(notation)
	parsed, err := rule.ParseHand(c)
	if err != nil || parsed.Type == rule.Invalid {
		panic(fmt.Sprintf("无法解析出牌记录 %q: %v", notation, err))
	}
	return PlayRecord{Played: parsed, IsLandlord: isLandlord}
}

// remaining 从牌面字符串构建记牌器（各点数剩余张数）
func remaining(notation string) map[card.Rank]int {
	m := make(map[card.Rank]int)
	for _, c := range cards(notation) {
		m[c.Rank]++
	}
	return m
}

// newTestEngine 从环境变量构建 Engine，如果没有 API Key 则跳过测试
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	_ = godotenv.Load("../../.env.local")

	apiKey := os.Getenv("AI_API_KEY")
	if apiKey == "" {
		t.Skip("AI_API_KEY 未设置，跳过 LLM 集成测试")
	}

	baseURL := os.Getenv("AI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := os.Getenv("AI_MODEL")
	if model == "" {
		model = "deepseek-v4-flash"
	}

	return NewEngine(config.AIConfig{
		Enabled:    true,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		MaxRetries: 3,
		Debug:      true,
	})
}

// --- 出牌测试场景 ---
//
// 场景按决策类型分类，统一选取中后期（残局/半残局）局面，此时记牌器信息充分、
// 大牌取舍与拦截时机最考验策略：
//   - 自由出牌(free)：手握出牌权，考察牌型规划与一波流意识
//   - 跟牌-顶牌(top) ：拆牌也要用大牌压制，阻止地主溜牌或拦截残血对手
//   - 跟牌-顺牌(follow)：用刚好压过的最小牌跟，保留大牌威慑
//   - 跟牌-炸弹/PASS(bomb_pass)：炸弹的取舍——该炸则炸，不该炸则保留或让队友

type playScenario struct {
	category string // free / top / follow / bomb_pass
	name     string
	gctx     GameContext
	expected string // 人类预期（用于对比，不影响测试通过）
}

func TestDecidePlay(t *testing.T) {
	engine := newTestEngine(t)

	scenarios := []playScenario{
		// ==================== 自由出牌 ====================
		{
			category: "free",
			name:     "地主残局_顺子+王炸一波流",
			gctx: GameContext{
				IsLandlord:     true,
				Hand:           cards("5 6 7 8 9 B R"),
				BottomCards:    cards("5 6 7"),
				RecentPlays:    [2]PlayRecord{},
				MustPlay:       true,
				CanBeat:        false,
				PlayerCounts:   [2]int{3, 4}, // 两农民只剩 3、4 张
				PlayerRoles:    [2]bool{false, false},
				RemainingCards: remaining("3 4 Q K A 2 J"),
			},
			expected: "先出顺子 5-9，下一手王炸收尾，本回合可锁定胜局；不应先拆王炸",
		},
		{
			category: "free",
			name:     "地主半残局_出长顺保留炸弹",
			gctx: GameContext{
				IsLandlord:     true,
				Hand:           cards("3 4 5 6 7 8 8 8 8 2"),
				BottomCards:    cards("9 6 4"),
				RecentPlays:    [2]PlayRecord{},
				MustPlay:       true,
				CanBeat:        false,
				PlayerCounts:   [2]int{7, 6},
				PlayerRoles:    [2]bool{false, false},
				RemainingCards: remaining("9 T J Q K A 2 3 5 7 9 J Q K"),
			},
			expected: "出顺子 3-7 破坏结构，保留炸弹 8888 与单 2；不应拆炸弹",
		},
		{
			category: "free",
			name:     "农民下家_队友传牌后自由跑顺子",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("7 8 9 T J Q 2"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("9 T J Q K", false), // 上家队友出顺子，地主已 pass
					{},
				},
				MustPlay:       true, // 地主 pass，轮到我自由出牌
				CanBeat:        false,
				PlayerCounts:   [2]int{5, 4}, // 上家队友 5 张，下家地主 4 张
				PlayerRoles:    [2]bool{false, true},
				RemainingCards: remaining("3 4 5 6 A A K 2 R 3 4"),
			},
			expected: "出顺子 7-Q 跑掉大部分手牌，只留单 2 收尾",
		},

		// ==================== 跟牌-顶牌 ====================
		{
			category: "top",
			name:     "地主仅剩2张_农民下家必须拦截",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("5 8 T Q 2 R"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("9", true), // 上家地主出单 9
					{},
				},
				MustPlay:       false,
				CanBeat:        true,
				PlayerCounts:   [2]int{2, 9}, // 上家地主仅剩 2 张！
				PlayerRoles:    [2]bool{true, false},
				RemainingCards: remaining("3 4 6 7 J K A 2 3 5 7"),
			},
			expected: "必须用单 2 压住拦截（留大王 R 防地主反扑），绝不能 pass",
		},
		{
			category: "top",
			name:     "地主溜小对_农民下家拆牌顶大对",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("4 4 7 7 9 J K K 2 2"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("5 5", true), // 上家地主溜小对 5
					{},
				},
				MustPlay:       false,
				CanBeat:        true,
				PlayerCounts:   [2]int{6, 9}, // 上家地主 6 张
				PlayerRoles:    [2]bool{true, false},
				RemainingCards: remaining("3 6 8 T Q A 3 6 8 T Q A"),
			},
			expected: "用对 2 顶住夺回出牌权（或对 K），阻止地主继续溜牌",
		},

		// ==================== 跟牌-顺牌 ====================
		{
			category: "follow",
			name:     "地主出小单_用最小单顺压保留大牌",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("6 8 9 J A 2 R"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("5", true), // 上家地主出单 5
					{},
				},
				MustPlay:       false,
				CanBeat:        true,
				PlayerCounts:   [2]int{8, 10},
				PlayerRoles:    [2]bool{true, false},
				RemainingCards: remaining("3 4 7 T Q K 3 4 7 T Q K"),
			},
			expected: "用单 6 顺着压即可，保留 A/2/大王等大牌；不应早早甩 2 或王",
		},
		{
			category: "follow",
			name:     "地主出中对_用刚好大的小对跟",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("8 8 9 9 J J A A 2"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("7 7", true), // 上家地主出对 7
					{},
				},
				MustPlay:       false,
				CanBeat:        true,
				PlayerCounts:   [2]int{9, 8},
				PlayerRoles:    [2]bool{true, false},
				RemainingCards: remaining("3 4 5 6 T Q K 3 4 5 6 T Q K"),
			},
			expected: "用对 8 跟（最小可压），保留 AA 与单 2；不应拆 AA 顶",
		},

		// ==================== 跟牌-炸弹/PASS ====================
		{
			category: "bomb_pass",
			name:     "队友出大牌地主已pass_保留炸弹放走队友",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("3 3 3 3 6 7 8"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("2 2", false), // 上家队友出对 2
					{},                 // 上上家地主已 pass
				},
				MustPlay:       false,
				CanBeat:        true,         // 手握炸弹 3333 可压，但不该用
				PlayerCounts:   [2]int{5, 8}, // 上家队友 5 张
				PlayerRoles:    [2]bool{false, true},
				RemainingCards: remaining("4 5 9 T J Q K A 2 R B 9"),
			},
			expected: "PASS——队友出大牌且地主已不要，绝不能用炸弹压队友",
		},
		{
			category: "bomb_pass",
			name:     "地主剩3张出对_果断炸弹拦截",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("4 4 4 4 7 9 T"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("K K", true), // 上家地主出对 K
					{},
				},
				MustPlay:       false,
				CanBeat:        true,
				PlayerCounts:   [2]int{3, 11}, // 上家地主仅剩 3 张
				PlayerRoles:    [2]bool{true, false},
				RemainingCards: remaining("3 5 6 8 J Q A 2 3 5 6"),
			},
			expected: "用炸弹 4444 拦截——地主≤3张随时跑完，必须果断炸",
		},
		{
			category: "bomb_pass",
			name:     "地主出对A手牌尚多_保留炸弹PASS",
			gctx: GameContext{
				IsLandlord:  false,
				Hand:        cards("5 5 5 5 6 7 8 9 T J"),
				BottomCards: nil,
				RecentPlays: [2]PlayRecord{
					play("A A", true), // 上家地主出对 A
					{},
				},
				MustPlay:       false,
				CanBeat:        true,          // 只能用炸弹 5555 压
				PlayerCounts:   [2]int{11, 9}, // 地主还有 11 张，不急
				PlayerRoles:    [2]bool{true, false},
				RemainingCards: remaining("3 4 Q K 2 B R 3 4 Q K 2"),
			},
			expected: "PASS——地主手牌尚多，炸弹留作威慑；手里 6-T 顺子可后续自由出",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.category+"/"+sc.name, func(t *testing.T) {
			result := engine.DecidePlay(context.Background(), "测试机器人", sc.gctx)

			var resultStr string
			if result == nil {
				resultStr = "PASS"
			} else {
				slices.SortFunc(result, func(a, b card.Card) int {
					return int(a.Rank) - int(b.Rank)
				})
				resultStr = cardsToStr(result)
			}
			t.Logf("========== 人类预期 ==========\n%s", sc.expected)
			t.Logf("========== LLM 结果 ==========\n%s", resultStr)
		})
	}
}

// --- 叫地主测试场景 ---

type bidScenario struct {
	name     string
	hand     []card.Card
	prevBid  *bool
	expected string
}

func TestDecideBid(t *testing.T) {
	engine := newTestEngine(t)

	scenarios := []bidScenario{
		{
			name:     "强牌_有炸弹和双王_应叫",
			hand:     cards("B R A A 2 2 K K K K J Q"),
			prevBid:  nil,
			expected: "true：炸弹+双王，极强手牌",
		},
		{
			name:     "弱牌_全散牌_不应叫",
			hand:     cards("3 4 5 7 8 9 T J Q"),
			prevBid:  nil,
			expected: "false：无大牌无炸弹，不应叫",
		},
		{
			name:     "上家已叫_手牌一般_不应抢",
			hand:     cards("3 5 7 9 J Q K A 3 4 6 8"),
			prevBid:  func() *bool { v := true; return &v }(),
			expected: "false：上家已叫，手牌不够强不应抢",
		},
		{
			name:     "上家未叫_手牌较强_应叫",
			hand:     cards("2 A A K K Q Q J J T T 9"),
			prevBid:  new(bool), // false
			expected: "true：双A双K多对子，手牌较强应叫",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Logf("\n========== 手牌 ==========\n%s", cardsToStr(sc.hand))
			t.Logf("========== 人类预期 ==========\n%s", sc.expected)

			result := engine.DecideBid(context.Background(), "测试机器人", sc.hand, sc.prevBid)
			t.Logf("========== LLM 结果 ==========\n%v", result)
		})
	}
}
