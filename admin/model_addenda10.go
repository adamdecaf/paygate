/*
 * Paygate Admin API
 *
 * Paygate is a RESTful API enabling Automated Clearing House ([ACH](https://en.wikipedia.org/wiki/Automated_Clearing_House)) transactions to be submitted and received without a deep understanding of a full NACHA file specification.
 *
 * API version: v1
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package admin

// Addenda10 struct for Addenda10
type Addenda10 struct {
	// Client defined string used as a reference to this record.
	Id string `json:"id,omitempty"`
	// 10 - NACHA regulations
	TypeCode string `json:"typeCode,omitempty"`
	// Transaction Type Code Describes the type of payment ANN = Annuity BUS = Business/Commercial DEP = Deposit LOA = Loan MIS = Miscellaneous MOR = Mortgage PEN = Pension RLS = Rent/Lease REM = Remittance2 SAL = Salary/Payroll TAX = Tax TEL = Telephone-Initiated Transaction WEB = Internet-Initiated Transaction ARC = Accounts Receivable Entry BOC = Back Office Conversion Entry POP = Point of Purchase Entry RCK = Re-presented Check Entry
	TransactionTypeCode string `json:"transactionTypeCode,omitempty"`
	// For inbound IAT payments this field should contain the USD amount or may be blank.
	ForeignPaymentAmount float32 `json:"foreignPaymentAmount,omitempty"`
	// Trace number
	ForeignTraceNumber string `json:"foreignTraceNumber,omitempty"`
	// Receiving Company Name/Individual Name
	Name string `json:"name,omitempty"`
	// EntryDetailSequenceNumber contains the ascending sequence number section of the Entry Detail or Corporate Entry Detail Record's trace number This number is the same as the last seven digits of the trace number of the related Entry Detail Record or Corporate Entry Detail Record.
	EntryDetailSequenceNumber float32 `json:"entryDetailSequenceNumber,omitempty"`
}