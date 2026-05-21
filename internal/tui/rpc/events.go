package rpc

type ResponseEvent struct {
	ResponseID int
	Message    any
}

type ResponseEventsClosed struct {
	ResponseID int
}

type ResponseCompleted struct {
	ResponseID int
	Text       string
	Err        error
}
