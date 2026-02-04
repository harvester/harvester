package log

import "log"

var (
	// Stupid log abstraction
	Infof = func(message string, obj ...interface{}) {
		log.Printf("INFO: "+message+"\n", obj...)
	}
	Errorf = func(message string, obj ...interface{}) {
		log.Printf("ERROR: "+message+"\n", obj...)
	}
	Debugf = func(message string, obj ...interface{}) {
		log.Printf("DEBUG: "+message+"\n", obj...)
	}
)
