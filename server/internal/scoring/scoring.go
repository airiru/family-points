// Package scoring 是**纯函数**计分逻辑：不碰数据库，便于单测。
//
// 把「档位怎么算、日总量怎么算」这类规则抽出来，是因为它们是全项目最容易算错的部分：
// repeat 按倍数循环、once 只在达标那一次发、多档独立且可同时命中。
package scoring

import (
	"encoding/json"
	"fmt"
	"sort"

	"familypoints/server/internal/model"
)

// Award 描述一次命中的档位奖励，落进 `records.streak_award` 便于追溯。
type Award struct {
	Every int    `json:"every"`
	N     int    `json:"n"`    // 命中时的连续天数
	Bonus int    `json:"bonus"`
	Mode  string `json:"mode"`
	From  string `json:"from"` // 本轮起点日期（YYYY-MM-DD），用于说明「哪一轮」
}

// DayPoints 计算日总量得分：≤ 阈值加分，> 阈值减分（双向，见 12.1）。
func DayPoints(totalMin, threshold, bonus, penalty int) int {
	if totalMin <= threshold {
		return bonus
	}
	return -penalty
}

// NightPenalty 计算「未按时睡觉」的扣分（2026-11-05 新增）。
//
// 口径：超出允许分钟数的部分**按单位向上取整**，每单位扣 points 分。
//   - unitMin 只能是 30 或 60（规则页只给这两个选项）
//   - 不满一个单位也按一个单位算（用了 31 分钟、单位 30 → 2 个单位）
//   - nightMin ≤ maxMin 时返回 0（达标不扣分）
//   - points ≤ 0 表示「不扣分」，只保持「不加分 + 连续清零」
//
// 例：允许 0 分钟、用了 100 分钟、单位 30、每单位 2 分 → ceil(100/30)=4 → -8
func NightPenalty(nightMin, maxMin, unitMin, points int) int {
	if points <= 0 {
		return 0
	}
	excess := nightMin - maxMin
	if excess <= 0 {
		return 0
	}
	unit := NormalizePenaltyUnit(unitMin)
	units := (excess + unit - 1) / unit // 向上取整
	return -units * points
}

// NormalizePenaltyUnit 把单位收敛到 30 / 60（非法值一律按 30 处理）。
func NormalizePenaltyUnit(unitMin int) int {
	if unitMin == 60 {
		return 60
	}
	return 30
}

// NightAwards 按连续天数计算命中的档位（可能多个）。
//
// 语义（7.3）：
//   - repeat：每满 N 天发一次（3、6、9…）
//   - once：只在连续天数**正好等于** N 时发一次；中断后重来会再发
//   - 多档独立计算，可同时命中
//
// roundStart 是本轮连续的第一天，用于在 streak_award 里记录「哪一轮」。
func NightAwards(streak int, tiers []model.StreakTier, roundStart string) []Award {
	if streak <= 0 || len(tiers) == 0 {
		return nil
	}
	out := make([]Award, 0, len(tiers))
	for _, t := range tiers {
		hit := false
		switch t.Mode {
		case "once":
			hit = streak == t.Every
		default: // repeat（含空值，默认 repeat）
			hit = t.Every > 0 && streak%t.Every == 0
		}
		if hit {
			out = append(out, Award{
				Every: t.Every, N: streak, Bonus: t.Bonus, Mode: t.Mode, From: roundStart,
			})
		}
	}
	return out
}

// ParseTiers 解析 rules.streaks（JSON 数组）。空字符串返回 nil。
func ParseTiers(raw *string) ([]model.StreakTier, error) {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil, nil
	}
	var tiers []model.StreakTier
	if err := json.Unmarshal([]byte(*raw), &tiers); err != nil {
		return nil, fmt.Errorf("streaks 不是合法 JSON: %w", err)
	}
	return tiers, nil
}

// ValidateTiers 校验档位配置（7.3 的约束）。
//
// 非法配置必须在**保存时**就拒绝，不能等到结算时才发现——
// 那时已经产生了错误的积分记录。
func ValidateTiers(tiers []model.StreakTier) error {
	if len(tiers) > 5 {
		return fmt.Errorf("档位最多 5 个，当前 %d 个", len(tiers))
	}
	seen := map[int]bool{}
	for i, t := range tiers {
		if t.Every < 1 {
			return fmt.Errorf("第 %d 档：every 必须 ≥ 1", i+1)
		}
		if t.Bonus == 0 {
			return fmt.Errorf("第 %d 档：bonus 不能为 0", i+1)
		}
		if t.Mode != "" && t.Mode != "repeat" && t.Mode != "once" {
			return fmt.Errorf("第 %d 档：mode 只能是 repeat 或 once", i+1)
		}
		if seen[t.Every] {
			return fmt.Errorf("every=%d 重复，同一组内档位天数不可重复", t.Every)
		}
		seen[t.Every] = true
	}
	return nil
}

// NormalizeTiers 把空 mode 补成 repeat，并按 every 升序排列（便于展示与比对）。
func NormalizeTiers(tiers []model.StreakTier) []model.StreakTier {
	out := make([]model.StreakTier, len(tiers))
	copy(out, tiers)
	for i := range out {
		if out[i].Mode == "" {
			out[i].Mode = "repeat"
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Every < out[j].Every })
	return out
}
