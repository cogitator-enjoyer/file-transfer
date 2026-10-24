package shared

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"

	"github.com/joho/godotenv"
)

type Int interface {
	int | uint | int8 | int16 | uint16
}

type Settings struct {
	SignalServerUrl string
}

func ParseAddrPort(addr string, port int) (netip.AddrPort, error) {
	return netip.ParseAddrPort(fmt.Sprintf("%s:%d", addr, port))
}

func GetSettings() (*Settings, error) {
	if err := godotenv.Load(".env"); err != nil {
		return nil, fmt.Errorf("error loading .env, err: %w", err)
	}
	return &Settings{
		SignalServerUrl: os.Getenv("SIGNAL_SERVER_URL"),
	}, nil
}

func GenerateRandomID(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
