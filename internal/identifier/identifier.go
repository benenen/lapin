package identifier

import (
	"fmt"

	"github.com/speps/go-hashids/v2"
)

const DefaultSalt = "lapin-development-salt"

type Codec struct {
	hashids *hashids.HashID
}

func New(salt string) (*Codec, error) {
	if salt == "" {
		salt = DefaultSalt
	}
	data := hashids.NewData()
	data.Salt = salt
	data.MinLength = 10
	codec, err := hashids.NewWithData(data)
	if err != nil {
		return nil, fmt.Errorf("create hashid codec: %w", err)
	}
	return &Codec{hashids: codec}, nil
}

func (c *Codec) Encode(id int64) string {
	encoded, err := c.hashids.EncodeInt64([]int64{id})
	if err != nil {
		panic(err)
	}
	return encoded
}

func (c *Codec) Decode(encoded string) (int64, error) {
	if len(encoded) == 0 || len(encoded) > 64 {
		return 0, fmt.Errorf("invalid hashid")
	}
	decoded, err := c.hashids.DecodeInt64WithError(encoded)
	if err != nil || len(decoded) != 1 || decoded[0] <= 0 {
		return 0, fmt.Errorf("invalid hashid")
	}
	return decoded[0], nil
}
