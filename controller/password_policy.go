package main

import (
	"errors"
	"fmt"
	"unicode"
)

const (
	minStrongPasswordLen = 10
	maxStrongPasswordLen = 16
)

const strongPasswordRule = "长度需 10-16 位，且至少包含 1 个大写字母、1 个小写字母、1 个数字、1 个特殊字符（如 !@#$%^&*_-+=），且不能包含空格"

func strongPasswordError() error {
	return fmt.Errorf("密码不符合强密码规则：%s", strongPasswordRule)
}

func validateStrongPassword(password string) error {
	runes := []rune(password)
	if len(runes) < minStrongPasswordLen || len(runes) > maxStrongPasswordLen {
		return strongPasswordError()
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range runes {
		if unicode.IsSpace(r) {
			return errors.New("密码不能包含空格")
		}
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return strongPasswordError()
	}
	return nil
}
