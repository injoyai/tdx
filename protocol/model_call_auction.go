package protocol

type callAuction struct{}

func (callAuction) Frame(code string) (*Frame, error) {
	exchange, number, err := DecodeCode(code)
	if err != nil {
		return nil, err
	}

	codeBs := []byte(number)
	return &Frame{
		Control: Control01,
		Type:    TypeCallAuction,
		Data:    append([]byte{exchange.Uint8(), 0x0}, codeBs...),
	}, nil
}

func (callAuction) Decode() {

}
