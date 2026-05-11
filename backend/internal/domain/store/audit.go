package store

import (
	"time"

	"github.com/google/uuid"
)

// FieldChange records one field's before and after values in an audit entry.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// AuditEntry is a row from store_audit_log joined with the user who made the change.
type AuditEntry struct {
	ID             uuid.UUID              `json:"id"`
	StoreID        uuid.UUID              `json:"store_id"`
	ChangedBy      uuid.UUID              `json:"changed_by"`
	ChangedByName  string                 `json:"changed_by_name"`
	ChangedByEmail string                 `json:"changed_by_email"`
	ChangeType     string                 `json:"change_type"`
	Changes        map[string]FieldChange `json:"changes"`
	CreatedAt      time.Time              `json:"created_at"`
}

// BuildDiff returns a map of field names → FieldChange for every field that
// differs between old and newer. Returns an empty map when nothing changed.
func BuildDiff(old, newer Store) map[string]FieldChange {
	diff := make(map[string]FieldChange)

	str := func(field, o, n string) {
		if o != n {
			diff[field] = FieldChange{Old: o, New: n}
		}
	}
	bl := func(field string, o, n bool) {
		if o != n {
			diff[field] = FieldChange{Old: o, New: n}
		}
	}
	ptr := func(field string, o, n *string) {
		ov, nv := "", ""
		if o != nil {
			ov = *o
		}
		if n != nil {
			nv = *n
		}
		if ov != nv {
			diff[field] = FieldChange{Old: ov, New: nv}
		}
	}

	str("name", old.Name, newer.Name)
	str("slug", old.Slug, newer.Slug)
	str("description", old.Description, newer.Description)
	str("logo_url", old.LogoURL, newer.LogoURL)
	bl("is_active", old.IsActive, newer.IsActive)
	str("trade_name", old.TradeName, newer.TradeName)
	str("phone", old.Phone, newer.Phone)
	str("address_zip", old.AddressZip, newer.AddressZip)
	str("address_street", old.AddressStreet, newer.AddressStreet)
	str("address_number", old.AddressNumber, newer.AddressNumber)
	str("address_complement", old.AddressComplement, newer.AddressComplement)
	str("address_neighborhood", old.AddressNeighborhood, newer.AddressNeighborhood)
	str("address_city", old.AddressCity, newer.AddressCity)
	str("address_state", old.AddressState, newer.AddressState)
	str("address_country", old.AddressCountry, newer.AddressCountry)
	ptr("legal_name", old.LegalName, newer.LegalName)
	ptr("document_number", old.DocumentNumber, newer.DocumentNumber)

	oldDT, newDT := "", ""
	if old.DocumentType != nil {
		oldDT = string(*old.DocumentType)
	}
	if newer.DocumentType != nil {
		newDT = string(*newer.DocumentType)
	}
	if oldDT != newDT {
		diff["document_type"] = FieldChange{Old: oldDT, New: newDT}
	}

	if old.DocumentStatus != newer.DocumentStatus {
		diff["document_status"] = FieldChange{
			Old: string(old.DocumentStatus),
			New: string(newer.DocumentStatus),
		}
	}

	if old.OwnerID != newer.OwnerID {
		diff["owner_id"] = FieldChange{Old: old.OwnerID.String(), New: newer.OwnerID.String()}
	}

	return diff
}
