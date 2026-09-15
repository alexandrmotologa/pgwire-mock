package protocol

// StartupMessage represents client initial connection parameters
type StartupMessage struct {
	ProtocolVersion int32
	Parameters      map[string]string
}

// User returns the authenticated user or empty string
func (s *StartupMessage) User() string {
	if s.Parameters == nil {
		return ""
	}
	return s.Parameters["user"]
}

// Database returns the database name or empty string
func (s *StartupMessage) Database() string {
	if s.Parameters == nil {
		return ""
	}
	return s.Parameters["database"]
}

// QueryMessage represents Simple Query ('Q')
type QueryMessage struct {
	Query string
}

// ParseMessage represents Extended Query Parse ('P')
type ParseMessage struct {
	StatementName string
	Query         string
	ParamOIDs     []int32
}

// BindMessage represents Extended Query Bind ('B')
type BindMessage struct {
	PortalName        string
	StatementName     string
	ParamFormats      []int16
	Parameters        [][]byte // nil slice element represents SQL NULL
	ResultFormatCodes []int16
}

// DescribeMessage represents Extended Query Describe ('D')
type DescribeMessage struct {
	TargetType byte // 'S' for prepared statement, 'P' for portal
	Name       string
}

// ExecuteMessage represents Extended Query Execute ('E')
type ExecuteMessage struct {
	PortalName string
	MaxRows    int32
}

// CloseMessage represents Extended Query Close ('C')
type CloseMessage struct {
	TargetType byte // 'S' for statement, 'P' for portal
	Name       string
}

// FieldDescription describes a single column in RowDescription ('T')
type FieldDescription struct {
	Name         string
	TableOID     int32
	ColumnAttr   int16
	DataTypeOID  int32
	DataTypeSize int16
	TypeModifier int32
	FormatCode   int16
}

// DefaultTypeSize returns sensible sizes for standard PostgreSQL types
func DefaultTypeSize(oid int32) int16 {
	switch oid {
	case OIDBool:
		return 1
	case OIDInt2:
		return 2
	case OIDInt4, OIDFloat4:
		return 4
	case OIDInt8, OIDFloat8, OIDTimestamp, OIDTimestamptz:
		return 8
	case OIDUUID:
		return 16
	default:
		return -1 // Variable length
	}
}

// ErrorField represents a key-value error field in ErrorResponse ('E')
type ErrorField struct {
	Type  byte
	Value string
}

// ErrorResponse represents an error packet to send to client
type ErrorResponse struct {
	Severity string
	Code     string // SQLSTATE 5-char code
	Message  string
	Detail   string
	Hint     string
}
