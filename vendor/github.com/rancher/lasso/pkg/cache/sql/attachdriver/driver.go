/*
Package attachdriver provides a sqlite driver that wraps any other driver and attaches a database at a given path when a connection is opened.
*/
package attachdriver

import (
	"database/sql"
	"database/sql/driver"
	"fmt"

	"modernc.org/sqlite"
)

// Name is the symbolic name for this driver in the sql framework to use with sql.Open
const Name = "attach_sqlite"

// AttachDriver uses an underlying driver to open the connection and attaches a database
// to that connection using path.
type AttachDriver struct {
	path string
	d    driver.Driver
}

// Register registers the attachdriver with the given path with go's sql package
func Register(path string) {
	d := &AttachDriver{
		path: path,
		d:    &sqlite.Driver{},
	}
	sql.Register(Name, d)
}

// Open opens a connection from the underlying driver and
// attaches a database as db2 using the underlying path.
func (ad AttachDriver) Open(name string) (driver.Conn, error) {
	c, err := ad.d.Open(name)
	if err != nil {
		return nil, err
	}

	stmt, err := c.Prepare("ATTACH DATABASE ? AS db2;")
	if err != nil {
		return nil, closeConnOnError(c, err)
	}

	_, err = stmt.Exec([]driver.Value{ad.path})
	if err != nil {
		stmt.Close()
		// cannot use defer close because stmt must be close before the following function closes the connection
		return nil, closeConnOnError(c, err)
	}
	stmt.Close()
	return c, nil
}

// closeConnOnError closes the sql.Rows object and wraps errors if needed
func closeConnOnError(c driver.Conn, err error) error {
	ce := c.Close()
	if ce != nil {
		return fmt.Errorf("failed to connection: %v, while handling error %w", ce, err)
	}

	return err
}
