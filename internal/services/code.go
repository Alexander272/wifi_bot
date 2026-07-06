package services

import (
	"crypto/rand"
	"math/big"
	"strings"
)

type CodeService struct{}

func NewCodeService() *CodeService {
	return &CodeService{}
}

const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (s *CodeService) Generate() string {
	b := make([]byte, 6)
	n := big.NewInt(int64(len(codeChars)))
	for i := range b {
		idx, _ := rand.Int(rand.Reader, n)
		b[i] = codeChars[idx.Int64()]
	}
	return string(b)
}

func (s *CodeService) IsValid(code string) bool {
	if len(code) != 6 {
		return false
	}
	code = strings.ToUpper(code)
	for _, ch := range code {
		if !strings.ContainsRune(codeChars, ch) {
			return false
		}
	}
	return true
}
