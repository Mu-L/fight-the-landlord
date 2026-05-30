package view

import (
	"strings"
	"testing"

	"github.com/palemoky/fight-the-landlord/internal/game/card"
)

// ranksOf 把重排后的牌拼成点数字符串，便于断言展示顺序
func ranksOf(cards []card.Card) string {
	var sb strings.Builder
	for _, c := range cards {
		sb.WriteString(c.Rank.String())
	}
	return sb.String()
}

func cardsFromRanks(ranks ...card.Rank) []card.Card {
	out := make([]card.Card, 0, len(ranks))
	for _, r := range ranks {
		out = append(out, card.Card{Rank: r})
	}
	return out
}

func TestGroupPlayedForDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   []card.Card
		want string
	}{
		{
			name: "单牌不变",
			in:   cardsFromRanks(card.Rank8),
			want: "8",
		},
		{
			name: "顺子按点数降序",
			in:   cardsFromRanks(card.Rank3, card.Rank4, card.Rank5, card.Rank6, card.Rank7),
			want: "76543",
		},
		{
			name: "三带一：主牌在前",
			in:   cardsFromRanks(card.Rank3, card.Rank3, card.Rank3, card.Rank8),
			want: "3338",
		},
		{
			name: "三带二：主牌在前",
			in:   cardsFromRanks(card.Rank4, card.Rank4, card.RankK, card.RankK, card.RankK),
			want: "KKK44",
		},
		{
			name: "三带一且附牌点数更大：仍主牌在前",
			in:   cardsFromRanks(card.Rank3, card.Rank3, card.Rank3, card.Rank2),
			want: "3332",
		},
		{
			name: "飞机带两对：主牌(三条)在前并降序",
			in:   cardsFromRanks(card.Rank5, card.Rank5, card.Rank5, card.Rank6, card.Rank6, card.Rank6, card.Rank8, card.Rank8, card.Rank3, card.Rank3),
			want: "6665558833",
		},
		{
			name: "四带两单：主牌(四条)在前",
			in:   cardsFromRanks(card.Rank3, card.Rank3, card.Rank3, card.Rank3, card.Rank9, card.Rank7),
			want: "333397",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ranksOf(groupPlayedForDisplay(tt.in))
			if got != tt.want {
				t.Errorf("groupPlayedForDisplay() = %q, want %q", got, tt.want)
			}
		})
	}
}
