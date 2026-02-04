package export

import (
	"github.com/guonaihong/gout/dataflow"
	"io"
	"os"
)

var _ dataflow.Curl = (*Curl)(nil)

type Curl struct {
	w               io.Writer
	df              *dataflow.DataFlow
	longOption      bool
	generateAndSend bool
}

func (c *Curl) New(df *dataflow.DataFlow) interface{} {
	return &Curl{df: df}
}

func (c *Curl) LongOption() dataflow.Curl {
	c.longOption = true
	return c
}

func (c *Curl) GenAndSend() dataflow.Curl {
	c.generateAndSend = true
	return c
}

func (c *Curl) SetOutput(w io.Writer) dataflow.Curl {
	c.w = w
	return c
}

func (c *Curl) Do() (err error) {
	if c.w == nil {
		c.w = os.Stdout
	}

	w := c.w

	req, err := c.df.Request()
	if err != nil {
		return err
	}

	client := c.df.Client()

	if c.generateAndSend {
		// 清空状态，Setxxx函数拆开使用就不会有问题
		defer c.df.Reset()
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		err = c.df.Bind(req, resp)
		if err != nil {
			return err
		}
	}

	return GenCurl(req, c.longOption, w)
}
