package dpi

import "testing"

func TestEnable_NoPanic(t *testing.T) { Enable() }
func TestEnable_Idempotent(t *testing.T) { Enable(); Enable(); Enable() }
