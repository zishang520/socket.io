package socket

type ExtendedError struct {
	message string
	data    any
}

func NewExtendedError(message string, data any) *ExtendedError {
	return &ExtendedError{message: message, data: data}
}

func (e *ExtendedError) Err() error {
	return e
}

func (e *ExtendedError) Data() any {
	return e.data
}

func (e *ExtendedError) Error() string {
	return e.message
}

type SeesionData struct {
	Pid    any
	Offset any
}

func (s *SeesionData) GetPid() (pid string, ok bool) {
	if s != nil && s.Pid != nil {
		switch _pid := s.Pid.(type) {
		case []string:
			if l := len(_pid); l > 0 {
				pid = _pid[l-1]
				ok = len(pid) > 0
			}
		case string:
			pid = _pid
			ok = len(pid) > 0
		}
	}
	return pid, ok
}

func (s *SeesionData) GetOffset() (offset string, ok bool) {
	if s != nil && s.Offset != nil {
		switch _offset := s.Offset.(type) {
		case []string:
			if l := len(_offset); l > 0 {
				offset = _offset[l-1]
				ok = len(offset) > 0
			}
		case string:
			offset = _offset
			ok = len(offset) > 0
		}
	}
	return offset, ok
}
