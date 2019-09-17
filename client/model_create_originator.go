/*
 * Paygate API
 *
 * Paygate is a RESTful API enabling Automated Clearing House ([ACH](https://en.wikipedia.org/wiki/Automated_Clearing_House)) transactions to be submitted and received without a deep understanding of a full NACHA file specification.
 *
 * API version: v1
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type CreateOriginator struct {
	// The depository account to be used by default per transfer. ID must be a valid Originator Depository account
	DefaultDepository string `json:"defaultDepository"`
	// An identification number by which the receiver is known to the originator.
	Identification string `json:"identification"`
	// Additional meta data to be used for display only
	Metadata string `json:"metadata,omitempty"`
}