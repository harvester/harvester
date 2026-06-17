package encode

import (
	"errors"
	"io"

	"github.com/guonaihong/gout/core"
	"github.com/guonaihong/gout/encoder"
	"google.golang.org/protobuf/proto"
)

var ErrNotImplMessage = errors.New("The proto.Message interface is not implemented")

type ProtoBufEncode struct {
	obj interface{}
}

func NewProtoBufEncode(obj interface{}) encoder.Encoder {
	if nil == obj {
		return nil
	}
	return &ProtoBufEncode{obj: obj}
}

func (p *ProtoBufEncode) Encode(w io.Writer) (err error) {
	if v, ok := core.GetBytes(p.obj); ok {
		//TODO找一个检测protobuf数据格式的函数
		_, err = w.Write(v)
		return err
	}

	var m proto.Message
	var ok bool

	m, ok = p.obj.(proto.Message)
	if !ok {
		// 这里如果能把普通结构体转成指针类型结构体就
		return ErrNotImplMessage
	}

	all, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	_, err = w.Write(all)
	return err
}

func (p *ProtoBufEncode) Name() string {
	return "protobuf"
}
