package widgets

type Element interface {
	Show() error
	Close() error
	GetData() (string, error)
	SetContent(string)
}
