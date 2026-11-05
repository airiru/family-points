package scoring

import (
	"testing"

	"familypoints/server/internal/model"
)

// 复刻文档 12.4 的示例序列：3 天 repeat +5、7 天 once +10。
func TestNightAwardsDocSequence(t *testing.T) {
	tiers := []model.StreakTier{
		{Every: 3, Bonus: 5, Mode: "repeat"},
		{Every: 7, Bonus: 10, Mode: "once"},
	}
	want := map[int]int{ // 连续天数 → 期望的档位奖励总额
		1: 0, 2: 0, 3: 5, 4: 0, 5: 0, 6: 5, 7: 10, 8: 0, 9: 5,
		14: 0, // 14 不是 3 的倍数；once(7) 本轮已发过，不再发
		15: 5, // 15 是 3 的倍数 → repeat 命中
		21: 5, // 21 同上：once 不再发
	}
	for n, expect := range want {
		sum := 0
		for _, a := range NightAwards(n, tiers, "2026-11-01") {
			sum += a.Bonus
		}
		if sum != expect {
			t.Errorf("连续 %d 天：期望 +%d，实际 +%d", n, expect, sum)
		}
	}
}

// 两档都是 repeat 时，第 21 天应同时命中 3 与 7。
func TestNightAwardsMultipleRepeat(t *testing.T) {
	tiers := []model.StreakTier{
		{Every: 3, Bonus: 5, Mode: "repeat"},
		{Every: 7, Bonus: 10, Mode: "repeat"},
	}
	awards := NightAwards(21, tiers, "2026-11-01")
	if len(awards) != 2 {
		t.Fatalf("第 21 天应命中 2 档，实际 %d 档", len(awards))
	}
	sum := 0
	for _, a := range awards {
		sum += a.Bonus
	}
	if sum != 15 {
		t.Errorf("期望 +15，实际 +%d", sum)
	}
}

// once 档中断重来后应能再发。
func TestNightAwardsOnceAfterReset(t *testing.T) {
	tiers := []model.StreakTier{{Every: 3, Bonus: 7, Mode: "once"}}
	if len(NightAwards(3, tiers, "a")) != 1 {
		t.Fatal("第 3 天应发 once 档")
	}
	if len(NightAwards(6, tiers, "a")) != 0 {
		t.Fatal("第 6 天同一轮内 once 不应再发")
	}
	// 中断后重新从 1 开始，第 3 天再次命中（新的 roundStart）
	if len(NightAwards(3, tiers, "b")) != 1 {
		t.Fatal("新一轮第 3 天应再次发放 once 档")
	}
}

func TestDayPoints(t *testing.T) {
	cases := []struct {
		total, threshold, bonus, penalty, want int
	}{
		{100, 120, 2, 3, 2},  // 未超阈值 → 加分
		{120, 120, 2, 3, 2},  // 正好等于阈值 → 算达标
		{121, 120, 2, 3, -3}, // 超 1 分钟 → 扣分
		{0, 120, 2, 3, 2},    // 完全没用 → 加分
	}
	for _, c := range cases {
		if got := DayPoints(c.total, c.threshold, c.bonus, c.penalty); got != c.want {
			t.Errorf("DayPoints(%d,%d)=%d，期望 %d", c.total, c.threshold, got, c.want)
		}
	}
}

func TestValidateTiers(t *testing.T) {
	bad := [][]model.StreakTier{
		{{Every: 0, Bonus: 5}},                       // every < 1
		{{Every: 3, Bonus: 0}},                       // bonus = 0
		{{Every: 3, Bonus: 5}, {Every: 3, Bonus: 7}}, // every 重复
		{{Every: 3, Bonus: 5, Mode: "weekly"}},       // 非法 mode
	}
	for i, tiers := range bad {
		if err := ValidateTiers(tiers); err == nil {
			t.Errorf("第 %d 组非法配置应被拒绝", i+1)
		}
	}
	good := []model.StreakTier{{Every: 3, Bonus: 5}, {Every: 7, Bonus: -2, Mode: "once"}}
	if err := ValidateTiers(good); err != nil {
		t.Errorf("合法配置被误拒: %v", err)
	}
}

// 未按时睡觉的扣分：超出部分按单位向上取整，每单位扣 N 分（2026-11-05）。
func TestNightPenalty(t *testing.T) {
	cases := []struct {
		name             string
		nightMin, maxMin int
		unitMin, points  int
		want             int
	}{
		{"没使用", 0, 0, 30, 2, 0},
		{"刚好达标", 30, 30, 30, 2, 0},
		{"超 1 分钟也算一个单位", 1, 0, 30, 2, -2},
		{"整 30 分钟一个单位", 30, 0, 30, 2, -2},
		{"31 分钟 → 2 个单位（向上取整）", 31, 0, 30, 2, -4},
		{"100 分钟 → 4 个单位", 100, 0, 30, 2, -8},
		{"单位改成 60：100 分钟 → 2 个单位", 100, 0, 60, 2, -4},
		{"允许 20 分钟：100 分钟超 80 → 3 个单位", 100, 20, 30, 5, -15},
		{"每单位 0 分 = 不扣分", 100, 0, 30, 0, 0},
		{"非法单位按 30 处理", 45, 0, 45, 1, -2},
	}
	for _, c := range cases {
		if got := NightPenalty(c.nightMin, c.maxMin, c.unitMin, c.points); got != c.want {
			t.Errorf("%s：期望 %d，实际 %d", c.name, c.want, got)
		}
	}
}
