package revdial

type Dialer struct {
	listen string
}

func NewDialer(listen string) *Dialer {
	return &Dialer{
		listen: listen,
	}
}

func (d *Dialer) Dial() error {
	return nil
}
