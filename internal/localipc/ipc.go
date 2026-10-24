package localipc

import (
	"net"

	"shellink/internal/config"
)

type Listener struct {
	NetListener net.Listener
	Token       string
	cleanup     func()
}

func Listen(cfg config.Config) (*Listener, error)      { return listenPlatform(cfg) }
func Dial(cfg config.Config) (net.Conn, string, error) { return dialPlatform(cfg) }
func (l *Listener) Close() error {
	err := l.NetListener.Close()
	if l.cleanup != nil {
		l.cleanup()
	}
	return err
}
