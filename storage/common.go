package storage

type byter interface {
	Bytes() []byte
}

func isEmpty(b byter) bool {
	return b == nil || len(b.Bytes()) == 0
}
