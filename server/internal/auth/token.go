// Package auth 负责签发与校验 Access Token（JWT）和 Refresh Token（随机串 + 哈希入库）。
//
// 两套 token 的职责严格分开：
//   - Access Token：无状态 JWT，2 小时，带 typ（admin / member），用于每个请求鉴权
//   - Refresh Token：随机 32 字节，30 天，只存 SHA-256 哈希，可主动吊销（改密/登出/轮换）
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"familypoints/server/internal/model"
)

var (
	// ErrInvalidToken 表示 token 无法解析、签名不对或已过期。
	ErrInvalidToken = errors.New("token 无效或已过期")
	// ErrWrongType 表示 typ 不符（拿前台 token 打后台接口，或反之）。
	ErrWrongType = errors.New("token 类型不匹配")
)

// Claims 是 Access Token 的载荷。注意：**角色不进 token**，
// 每次请求从数据库读当前配置，避免「改了组长但旧 token 还能用」。
type Claims struct {
	Sub uint64 `json:"sub"`
	Typ string `json:"typ"`
	jwt.RegisteredClaims
}

// Manager 持有签名密钥与有效期。
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewManager 构造 Manager。secret 为空时直接报错——不允许无签名运行。
func NewManager(secret string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	if secret == "" {
		return nil, errors.New("JWT_SECRET 未配置")
	}
	return &Manager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}, nil
}

// AccessTTL / RefreshTTL 暴露给上层，用于返回给客户端的过期时间。
func (m *Manager) AccessTTL() time.Duration  { return m.accessTTL }
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

// SignAccess 签发 Access Token。
func (m *Manager) SignAccess(subjectID uint64, typ string) (string, error) {
	now := time.Now()
	claims := Claims{
		Sub: subjectID,
		Typ: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// ParseAccess 解析并校验 Access Token，并检查 typ 是否为期望值。
func (m *Manager) ParseAccess(raw, wantTyp string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名算法: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}
	if claims.Typ != wantTyp {
		return nil, ErrWrongType
	}
	return claims, nil
}

// NewRefresh 生成 refresh token 原文与哈希。原文只在这一次返回给客户端。
func NewRefresh() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	return raw, HashRefresh(raw), nil
}

// HashRefresh 计算 refresh token 的存储形态。
func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// SubjectTypeOf 把角色域映射成 subject_type 值，避免各处硬编码字符串。
func SubjectTypeOf(kind string) string {
	if kind == model.SubjectAdmin {
		return model.SubjectAdmin
	}
	return model.SubjectMember
}
