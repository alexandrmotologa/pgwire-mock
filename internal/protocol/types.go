package protocol

// Protocol version constants
const (
	ProtocolVersion3 = 196608   // 3.0 (major 3, minor 0)
	SSLRequestCode   = 80877103 // 1234.5679
	CancelRequestCode = 80877102 // 1234.5678
)

// Frontend message types (received from client)
const (
	MsgTypeQuery            byte = 'Q'
	MsgTypeParse            byte = 'P'
	MsgTypeBind             byte = 'B'
	MsgTypeDescribe         byte = 'D'
	MsgTypeExecute          byte = 'E'
	MsgTypeSync             byte = 'S'
	MsgTypeFlush            byte = 'H'
	MsgTypeClose            byte = 'C'
	MsgTypeTerminate        byte = 'X'
	MsgTypePasswordResponse byte = 'p'
)

// Backend message types (sent to client)
const (
	MsgTypeAuth                 byte = 'R'
	MsgTypeBackendKeyData       byte = 'K'
	MsgTypeParameterStatus      byte = 'S'
	MsgTypeReadyForQuery        byte = 'Z'
	MsgTypeRowDescription       byte = 'T'
	MsgTypeDataRow              byte = 'D'
	MsgTypeCommandComplete      byte = 'C'
	MsgTypeErrorResponse        byte = 'E'
	MsgTypeNoticeResponse       byte = 'N'
	MsgTypeParseComplete        byte = '1'
	MsgTypeBindComplete         byte = '2'
	MsgTypeCloseComplete        byte = '3'
	MsgTypeNoData               byte = 'n'
	MsgTypeParameterDescription byte = 't'
	MsgTypeEmptyQueryResponse   byte = 'I'
)

// Transaction status indicators for ReadyForQuery
const (
	TxStatusIdle        byte = 'I' // Idle, not in a transaction block
	TxStatusInBlock     byte = 'T' // In a transaction block
	TxStatusFailedBlock byte = 'E' // In a failed transaction block
)

// Standard PostgreSQL Data Type OIDs
const (
	OIDBool        int32 = 16
	OIDBytea       int32 = 17
	OIDInt8        int32 = 20
	OIDInt2        int32 = 21
	OIDInt4        int32 = 23
	OIDText        int32 = 25
	OIDJSON        int32 = 114
	OIDXML         int32 = 142
	OIDFloat4      int32 = 700
	OIDFloat8      int32 = 701
	OIDVarchar     int32 = 1043
	OIDDate        int32 = 1082
	OIDTime        int32 = 1083
	OIDTimestamp   int32 = 1114
	OIDTimestamptz int32 = 1184
	OIDInterval    int32 = 1186
	OIDNumeric     int32 = 1700
	OIDUUID        int32 = 2950
	OIDJSONB       int32 = 3802
)

// Format codes
const (
	FormatText   int16 = 0
	FormatBinary int16 = 1
)

// Error field identifiers for ErrorResponse
const (
	ErrorSeverityNonLocalized byte = 'V'
	ErrorSeverity             byte = 'S'
	ErrorCode                 byte = 'C' // SQLSTATE code
	ErrorMessage              byte = 'M'
	ErrorDetail               byte = 'D'
	ErrorHint                 byte = 'H'
	ErrorPosition             byte = 'P'
	ErrorInternalPosition     byte = 'p'
	ErrorInternalQuery        byte = 'q'
	ErrorWhere                byte = 'W'
	ErrorSchema               byte = 's'
	ErrorTable                byte = 't'
	ErrorColumn               byte = 'c'
	ErrorDataType             byte = 'd'
	ErrorConstraint           byte = 'n'
	ErrorFile                 byte = 'F'
	ErrorLine                 byte = 'L'
	ErrorRoutine              byte = 'R'
)
