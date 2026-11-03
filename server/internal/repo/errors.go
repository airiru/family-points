package repo

import "errors"

// 账目相关的仓储层错误。
var (
	// ErrInsufficient 余额不足（事务内校验失败）。
	ErrInsufficient = errors.New("积分不足")
)
