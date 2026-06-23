package services

import (
	"math/rand"
	"strings"
	"time"
)

type CodeService struct{}

func NewCodeService() *CodeService {
	return &CodeService{}
}

const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (s *CodeService) Generate() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 6)
	for i := range b {
		b[i] = codeChars[r.Intn(len(codeChars))]
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
