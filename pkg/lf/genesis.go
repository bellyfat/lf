/*
 * LF: Global Fully Replicated Key/Value Store
 * Copyright (C) 2018-2019  ZeroTier, Inc.  https://www.zerotier.com/
 *
 * Licensed under the terms of the MIT license (see LICENSE.txt).
 */

package lf

import (
	"encoding/json"
	"strings"
)

// GenesisParameters is the payload (JSON encoded) of the first RecordMinLinks records in a global data store.
type GenesisParameters struct {
	initialized bool

	Name                       string   `json:",omitempty"` // Name of this LF network / data store
	Contact                    string   `json:",omitempty"` // Contact info for this network (may be empty)
	Comment                    string   `json:",omitempty"` // Optional comment
	RootCertificateAuthorities []Blob   `json:",omitempty"` // X.509 certificates for master CAs for this data store (empty for an unbiased work-only data store)
	CertificateRequired        bool     `json:""`           // Is a certificate required? (must be false if there are no CAs, obviously)
	WorkRequired               bool     `json:""`           // Is proof of work required?
	LinkKey                    [32]byte `json:""`           // Static 32-byte key used to ensure that nodes in this network only connect to one another
	TimestampFloor             uint64   `json:""`           // Floor for network record timestamps (seconds)
	RecordMinLinks             uint     `json:""`           // Minimum number of links required for non-genesis records
	RecordMaxValueSize         uint     `json:""`           // Maximum size of record values
	RecordMaxSize              uint     `json:""`           // Maximum size of records (up to the RecordMaxSize constant)
	RecordMaxForwardTimeDrift  uint     `json:""`           // Maximum number of seconds in the future a record can be timestamped
	AmendableFields            []string `json:",omitempty"` // List of json field names that the genesis owner can change by posting non-empty records
}

// Update updates these GenesisParameters from a JSON encoded parameter set.
// This handles the initial update and then constraining later updated by AmendableFields and which fields are present.
func (gp *GenesisParameters) Update(jsonValue []byte) error {
	if len(jsonValue) == 0 {
		return nil
	}

	updFields := make(map[string]*json.RawMessage)
	err := json.Unmarshal(jsonValue, &updFields)
	if err != nil {
		return err
	}
	var ngp GenesisParameters
	err = json.Unmarshal(jsonValue, &ngp)
	if err != nil {
		return err
	}

	afields := gp.AmendableFields
	for k := range updFields {
		skip := gp.initialized
		if skip {
			for _, af := range afields {
				if strings.EqualFold(af, k) {
					skip = false
					break
				}
			}
		}
		if !skip {
			switch strings.ToLower(k) {
			case "name":
				gp.Name = ngp.Name
			case "contact":
				gp.Contact = ngp.Contact
			case "comment":
				gp.Comment = ngp.Comment
			case "rootcertificateauthorities":
				gp.RootCertificateAuthorities = ngp.RootCertificateAuthorities
			case "certificaterequired":
				gp.CertificateRequired = ngp.CertificateRequired
			case "workrequired":
				gp.WorkRequired = ngp.WorkRequired
			case "linkkey":
				gp.LinkKey = ngp.LinkKey
			case "timestampfloor":
				gp.TimestampFloor = ngp.TimestampFloor
			case "recordminlinks":
				gp.RecordMinLinks = ngp.RecordMinLinks
			case "recordmaxvaluesize":
				gp.RecordMaxValueSize = ngp.RecordMaxValueSize
			case "recordmaxsize":
				gp.RecordMaxSize = ngp.RecordMaxSize
			case "recordmaxforwardtimedrift":
				gp.RecordMaxForwardTimeDrift = ngp.RecordMaxForwardTimeDrift
			case "amendablefields":
				gp.AmendableFields = ngp.AmendableFields
			}
		}
	}
	gp.initialized = true

	return nil
}

// CreateGenesisRecords creates a set of genesis records for a new LF data store.
// The number created is always sufficient to satisfy RecordMinLinks for subsequent records.
// If RecordMinLinks is zero one record is created. The first genesis record will contain
// the Genesis parameters in JSON format while subsequent records are empty.
func CreateGenesisRecords(genesisOwnerType int, genesisParameters *GenesisParameters) ([]*Record, *Owner, error) {
	gpjson, err := json.Marshal(genesisParameters)
	if err != nil {
		return nil, nil, err
	}

	var records []*Record
	var links [][]byte
	genesisOwner, err := NewOwner(genesisOwnerType)
	if err != nil {
		return nil, nil, err
	}
	now := TimeSec()

	var wg *Wharrgarblr
	if genesisParameters.WorkRequired {
		wg = NewWharrgarblr(RecordDefaultWharrgarblMemory, 0)
	}

	// Create the very first genesis record, which contains the genesis configuration structure in JSON format.
	r, err := NewRecord(gpjson, nil, nil, nil, nil, nil, now, wg, 0, genesisOwner)
	if err != nil {
		return nil, nil, err
	}
	records = append(records, r)
	links = append(links, r.Hash()[:])

	// Subsequent genesis records are empty and just exist so real records can satisfy their minimum link requirement.
	for i := uint(1); i < genesisParameters.RecordMinLinks; i++ {
		r, err := NewRecord(nil, links, nil, nil, nil, nil, now+uint64(i), wg, 0, genesisOwner)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, r)
		links = append(links, r.Hash()[:])
	}

	return records, genesisOwner, nil
}

//////////////////////////////////////////////////////////////////////////////

// A globally shared data store for Earth and its neighbors in the Sol system.
// Should theoretically be useful up to Kardashev Type II civilization scale.

/*
{
  "Name": "Sol",
  "Comment": "Global Public LF Data Store",
  "CertificateRequired": false,
  "WorkRequired": true,
  "LinkKey": [100, 231, 222, 7, 48, 31, 251, 135, 67, 137, 212, 187, 223, 10, 152, 159, 153, 55, 193, 205, 6, 218, 77, 17, 72, 44, 112, 225, 14, 229, 243, 0],
  "TimestampFloor": 1551399635,
  "RecordMinLinks": 3,
  "RecordMaxValueSize": 1024,
  "RecordMaxSize": 65536,
  "RecordMaxForwardTimeDrift": 15
}
*/

// SolGenesisRecords are the genesis records for the "Sol" LF network, the global shared LF instance.
// Sol is work-only and does not permit revisions by the genesis owner, making its configuration effectively set in stone without code changes.
var SolGenesisRecords = []byte{0x0, 0x0, 0xff, 0x1, 0x1, 0xb4, 0x82, 0x7b, 0x44, 0x38, 0x9, 0xd3, 0xa6, 0x8c, 0x8, 0x1d, 0x22, 0xa6, 0xbc, 0x61, 0x23, 0x82, 0x85, 0x88, 0x21, 0x6f, 0xda, 0x18, 0x74, 0x43, 0x7, 0xa1, 0x88, 0x23, 0x6c, 0xde, 0x88, 0x9, 0xc3, 0x6, 0x4, 0x94, 0x3a, 0x62, 0xd8, 0xa4, 0x19, 0x3, 0x82, 0x89, 0x11, 0x10, 0x44, 0xc2, 0xd0, 0x9, 0x3, 0x62, 0xa, 0x9d, 0x37, 0x72, 0xe, 0x3e, 0x1c, 0x52, 0x46, 0xe, 0x9d, 0x34, 0x66, 0x48, 0xae, 0x2c, 0x23, 0xa5, 0x4c, 0x9c, 0x3a, 0x69, 0x64, 0x92, 0x41, 0x68, 0xa6, 0xe3, 0x9c, 0x32, 0xf, 0xaf, 0xc4, 0x5c, 0xe3, 0x13, 0xa8, 0xd0, 0x32, 0x44, 0x75, 0xd0, 0x91, 0x53, 0x27, 0xa9, 0x8, 0x26, 0x69, 0xdc, 0xac, 0x59, 0x52, 0x26, 0xcf, 0x45, 0x2e, 0x62, 0xb4, 0x3c, 0x31, 0x53, 0x46, 0x88, 0x9e, 0x20, 0x66, 0x56, 0xd0, 0x20, 0x43, 0x24, 0xd, 0x99, 0x29, 0x37, 0x66, 0xdc, 0x89, 0x93, 0xc5, 0x4d, 0x8d, 0x35, 0x73, 0xc7, 0xc0, 0x38, 0x22, 0x63, 0xd, 0xc, 0x29, 0x53, 0x86, 0xe0, 0xb9, 0x43, 0x23, 0xca, 0xd, 0x36, 0x38, 0xee, 0x4, 0xe9, 0xe1, 0x50, 0x4, 0x95, 0x34, 0x6, 0xe7, 0xb0, 0x6c, 0x3, 0xc7, 0x88, 0xc6, 0x98, 0x8, 0x63, 0xd4, 0xa8, 0x11, 0x63, 0x46, 0x8e, 0x1c, 0x36, 0x66, 0xd4, 0x78, 0xe8, 0x73, 0x4c, 0x4c, 0x32, 0x4d, 0xb4, 0x66, 0xdd, 0x3a, 0x7, 0xe1, 0x8c, 0xd2, 0x65, 0x4e, 0xcb, 0x49, 0x1d, 0x6, 0x8f, 0x95, 0x8e, 0x56, 0xa7, 0xa4, 0xd1, 0x73, 0x50, 0x47, 0xc, 0x18, 0x32, 0x68, 0xc4, 0x9e, 0x5d, 0x1b, 0xcf, 0xee, 0xde, 0x8, 0x6d, 0x70, 0x9e, 0x61, 0x83, 0x38, 0xea, 0x26, 0xb6, 0x8d, 0xc4, 0xbc, 0x13, 0x86, 0x36, 0x64, 0x83, 0x44, 0xe4, 0xe4, 0xb4, 0xf8, 0xbb, 0x46, 0x9f, 0x80, 0x20, 0x50, 0x79, 0x23, 0xb2, 0xde, 0xb7, 0x12, 0xc8, 0x51, 0x1a, 0xc, 0x7a, 0x33, 0x65, 0x16, 0x8, 0xd3, 0xed, 0x1, 0xc3, 0xef, 0xec, 0xdf, 0x4, 0x82, 0x36, 0x72, 0x2, 0xae, 0xb0, 0x3f, 0xcc, 0x0, 0xd3, 0xf5, 0xe1, 0xe3, 0x5, 0x0, 0x1, 0x5, 0xd0, 0x5c, 0xbb, 0xe5, 0x10, 0x28, 0x1e, 0xa3, 0x45, 0x0, 0x1, 0x1d, 0x66, 0x40, 0xb, 0x8b, 0xa8, 0xb2, 0xf3, 0xa3, 0x71, 0xe3, 0xbc, 0xcf, 0x36, 0xe1, 0x56, 0xd1, 0x5e, 0x6d, 0x81, 0xe0, 0xf7, 0xdb, 0xf3, 0x91, 0xaf, 0x92, 0x39, 0xf2, 0x80, 0x60, 0x88, 0x65, 0x9e, 0x1c, 0xf1, 0xe2, 0x2d, 0x1a, 0xf6, 0x70, 0xc5, 0x1e, 0x7b, 0x7, 0x95, 0x15, 0x98, 0xf, 0x28, 0xa2, 0x3f, 0x41, 0x8b, 0x88, 0x9e, 0xad, 0x7a, 0xc1, 0x36, 0xcb, 0xbb, 0xcb, 0x15, 0x6d, 0x1, 0xe, 0x0, 0x0, 0x0, 0x20, 0x50, 0x79, 0x23, 0xb2, 0xde, 0xb7, 0x12, 0xc8, 0x51, 0x1a, 0xc, 0x7a, 0x33, 0x65, 0x16, 0x8, 0xd3, 0xed, 0x1, 0xc3, 0xef, 0xec, 0xdf, 0x4, 0x82, 0x36, 0x72, 0x2, 0xae, 0xb0, 0x3f, 0xcc, 0x1, 0x9, 0x5c, 0x8f, 0xc2, 0xd5, 0xba, 0x7d, 0x3e, 0x3c, 0x8d, 0x41, 0x1, 0x80, 0x3a, 0x24, 0x18, 0xc3, 0xf3, 0x6f, 0xf0, 0x6c, 0xab, 0x23, 0x19, 0xf3, 0xf2, 0x9a, 0xcb, 0xba, 0x9e, 0x58, 0x1e, 0xd4, 0xf5, 0xe1, 0xe3, 0x5, 0x0, 0x1, 0x24, 0x92, 0x1a, 0xad, 0xe0, 0x1f, 0x28, 0x8a, 0x2e, 0x45, 0x0, 0x0, 0x1f, 0xa7, 0x40, 0xda, 0x8b, 0xfc, 0x5a, 0x2a, 0xcf, 0x82, 0xbf, 0x8c, 0x44, 0xb5, 0xca, 0x64, 0x2e, 0xfb, 0xa2, 0x37, 0x15, 0xcf, 0x4a, 0x3c, 0x24, 0xd0, 0xf6, 0xb2, 0xf6, 0x40, 0x42, 0xd7, 0x99, 0xbf, 0x8b, 0xeb, 0x65, 0x9d, 0xc6, 0xe4, 0x19, 0x6, 0x4b, 0xdd, 0x7b, 0x8d, 0xc4, 0x23, 0x23, 0x62, 0x57, 0x12, 0xae, 0x88, 0x3c, 0x57, 0x4, 0x30, 0xb7, 0x92, 0xfd, 0xa4, 0xc, 0x82, 0x3b, 0x3b, 0x0, 0x0, 0x0, 0x0, 0x20, 0x50, 0x79, 0x23, 0xb2, 0xde, 0xb7, 0x12, 0xc8, 0x51, 0x1a, 0xc, 0x7a, 0x33, 0x65, 0x16, 0x8, 0xd3, 0xed, 0x1, 0xc3, 0xef, 0xec, 0xdf, 0x4, 0x82, 0x36, 0x72, 0x2, 0xae, 0xb0, 0x3f, 0xcc, 0x2, 0x9, 0x5c, 0x8f, 0xc2, 0xd5, 0xba, 0x7d, 0x3e, 0x3c, 0x8d, 0x41, 0x1, 0x80, 0x3a, 0x24, 0x18, 0xc3, 0xf3, 0x6f, 0xf0, 0x6c, 0xab, 0x23, 0x19, 0xf3, 0xf2, 0x9a, 0xcb, 0xba, 0x9e, 0x58, 0x1e, 0x10, 0x63, 0x79, 0x4f, 0x66, 0xe9, 0x48, 0x87, 0xa5, 0xd2, 0x88, 0x8c, 0xee, 0x16, 0x5f, 0x2d, 0x60, 0x9f, 0x1, 0x7c, 0xac, 0xf5, 0x60, 0xe7, 0x6e, 0xb2, 0x92, 0x72, 0x75, 0xa4, 0x49, 0x48, 0xd5, 0xf5, 0xe1, 0xe3, 0x5, 0x0, 0x1, 0x3d, 0x87, 0xda, 0xff, 0x55, 0x15, 0x8a, 0x8b, 0x9b, 0xd0, 0x0, 0x0, 0x38, 0x9a, 0x40, 0x99, 0xc0, 0xca, 0x76, 0x48, 0x26, 0x8f, 0x8f, 0x4b, 0x64, 0xd8, 0x4c, 0xf, 0x87, 0x23, 0x1d, 0x81, 0x24, 0xa2, 0xa0, 0xb1, 0x67, 0xe7, 0x29, 0xf3, 0x39, 0xea, 0xcc, 0x49, 0xec, 0xed, 0x49, 0xe1, 0xc6, 0x9, 0xfd, 0x6, 0xff, 0x5f, 0xae, 0xa2, 0xb3, 0x64, 0x1e, 0x7, 0xff, 0x2d, 0x62, 0x31, 0x8b, 0x80, 0xc6, 0x31, 0x48, 0xb4, 0xbe, 0xa8, 0x16, 0x22, 0x7b, 0x85, 0xdf, 0xd8, 0x7}
