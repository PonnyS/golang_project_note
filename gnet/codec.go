package gnet

type (
	ICodec interface {
		Encode(c Conn, buf []byte) ([]byte, error)
		Decode(c Conn) ([]byte, error)
	}
)
