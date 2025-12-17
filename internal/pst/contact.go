package pst

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
	"unicode/utf16"

	"github.com/emersion/go-vcard"
	"github.com/mooijtech/go-pst/v6/pkg"
	"github.com/mooijtech/go-pst/v6/pkg/properties"
)

// Contact represents a contact ready for upload
type Contact struct {
	UID  string     // Unique ID for CardDAV
	Name string     // Display name for logging
	Card vcard.Card // vCard data
}

// ContactCallback is called for each contact as it's read from the PST
// Return an error to stop processing
type ContactCallback func(contact *Contact) error

// Named property IDs for email addresses in PSETID_Address namespace
// See MS-OXPROPS: PidLidEmail1EmailAddress, PidLidEmail2EmailAddress, PidLidEmail3EmailAddress
const (
	pidLidEmail1EmailAddress = 0x8083
	pidLidEmail2EmailAddress = 0x8093
	pidLidEmail3EmailAddress = 0x80A3
)

// readNamedProperty reads a named property from the PropertyContext using NameToIDMap
// Returns empty string if property not found or error occurs
func readNamedProperty(pstFile *pst.File, propContext *pst.PropertyContext, localDescriptors []pst.LocalDescriptor, namedPropID int) string {
	// Map the named property ID to actual property ID
	mappedID, err := pstFile.NameToIDMap.GetPropertyID(namedPropID, pst.PropertySetAddress)
	if err != nil {
		return ""
	}

	// Read the property value
	propReader, err := propContext.GetPropertyReader(uint16(mappedID), localDescriptors)
	if err != nil {
		return ""
	}

	// Read raw data
	data := make([]byte, propReader.Size())
	_, err = propReader.ReadAt(data, 0)
	if err != nil {
		return ""
	}

	// Decode UTF-16LE string
	return decodeUTF16LE(data)
}

// decodeUTF16LE decodes a UTF-16 little-endian byte slice to a Go string
func decodeUTF16LE(data []byte) string {
	if len(data) < 2 {
		return ""
	}

	// Convert bytes to uint16 slice
	u16s := make([]uint16, len(data)/2)
	for i := 0; i < len(u16s); i++ {
		u16s[i] = binary.LittleEndian.Uint16(data[i*2:])
	}

	// Decode UTF-16 to runes, then to string
	runes := utf16.Decode(u16s)
	return string(runes)
}

// buildContact converts PST contact properties to a Contact with vCard
// Returns nil if the contact has no useful data
func buildContact(pstFile *pst.File, propContext *pst.PropertyContext, localDescriptors []pst.LocalDescriptor, props *properties.Contact, msgProps *properties.Message) *Contact {
	// Get name components
	givenName := props.GetGivenName()
	surname := props.GetSurname()
	fileUnder := props.GetFileUnder()

	// Build display name
	var displayName string
	if givenName != "" && surname != "" {
		displayName = givenName + " " + surname
	} else if givenName != "" {
		displayName = givenName
	} else if surname != "" {
		displayName = surname
	} else if fileUnder != "" {
		displayName = fileUnder
	}

	// Get email addresses via named properties (go-pst Contact methods don't work for these)
	// Email addresses are stored as named properties in PSETID_Address namespace
	email1 := readNamedProperty(pstFile, propContext, localDescriptors, pidLidEmail1EmailAddress)
	email2 := readNamedProperty(pstFile, propContext, localDescriptors, pidLidEmail2EmailAddress)
	email3 := readNamedProperty(pstFile, propContext, localDescriptors, pidLidEmail3EmailAddress)

	// Skip contacts with no name and no email
	if displayName == "" && email1 == "" && email2 == "" && email3 == "" {
		return nil
	}

	// Use email as fallback display name
	if displayName == "" {
		if email1 != "" {
			displayName = email1
		} else if email2 != "" {
			displayName = email2
		} else if email3 != "" {
			displayName = email3
		}
	}

	// Generate UID from content hash for deduplication
	uid := generateContactUID(displayName, email1, email2, email3)

	// Build vCard
	card := buildVCard(props, msgProps, displayName, givenName, surname, uid, email1, email2, email3)

	return &Contact{
		UID:  uid,
		Name: displayName,
		Card: card,
	}
}

// generateContactUID creates a unique ID based on contact data
func generateContactUID(name, email1, email2, email3 string) string {
	data := name + "|" + email1 + "|" + email2 + "|" + email3
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
}

// buildVCard constructs a vcard.Card from contact properties
func buildVCard(props *properties.Contact, msgProps *properties.Message, displayName, givenName, surname, uid, email1, email2, email3 string) vcard.Card {
	card := make(vcard.Card)

	// VERSION is required
	card.SetValue(vcard.FieldVersion, "3.0")

	// UID
	card.SetValue(vcard.FieldUID, uid+"@pst-import")

	// FN (formatted name) - required
	card.SetValue(vcard.FieldFormattedName, displayName)

	// N (structured name)
	// Format: Family;Given;Additional;Prefix;Suffix
	prefix := props.GetDisplayNamePrefix()
	card.Set(vcard.FieldName, &vcard.Field{
		Value: surname + ";" + givenName + ";;" + prefix + ";",
	})

	// Email addresses (passed in as parameters, read via named properties)
	if email1 != "" {
		card.Add(vcard.FieldEmail, &vcard.Field{
			Value:  email1,
			Params: vcard.Params{vcard.ParamType: {"INTERNET"}},
		})
	}
	if email2 != "" {
		card.Add(vcard.FieldEmail, &vcard.Field{
			Value:  email2,
			Params: vcard.Params{vcard.ParamType: {"INTERNET"}},
		})
	}
	if email3 != "" {
		card.Add(vcard.FieldEmail, &vcard.Field{
			Value:  email3,
			Params: vcard.Params{vcard.ParamType: {"INTERNET"}},
		})
	}

	// Phone numbers from Contact properties
	if phone := props.GetBusinessTelephoneNumber(); phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  phone,
			Params: vcard.Params{vcard.ParamType: {"WORK", "VOICE"}},
		})
	}
	if phone := props.GetBusiness2TelephoneNumber(); phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  phone,
			Params: vcard.Params{vcard.ParamType: {"WORK", "VOICE"}},
		})
	}
	if phone := props.GetHomeTelephoneNumber(); phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  phone,
			Params: vcard.Params{vcard.ParamType: {"HOME", "VOICE"}},
		})
	}
	if phone := props.GetHome2TelephoneNumber(); phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  phone,
			Params: vcard.Params{vcard.ParamType: {"HOME", "VOICE"}},
		})
	}
	if phone := props.GetPrimaryTelephoneNumber(); phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  phone,
			Params: vcard.Params{vcard.ParamType: {"PREF"}},
		})
	}
	if phone := props.GetCarTelephoneNumber(); phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  phone,
			Params: vcard.Params{vcard.ParamType: {"CAR"}},
		})
	}
	if phone := props.GetCompanyMainTelephoneNumber(); phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  phone,
			Params: vcard.Params{vcard.ParamType: {"WORK"}},
		})
	}

	// Phone numbers from Message properties (mobile, pager, other)
	if msgProps != nil {
		if phone := msgProps.GetMobileTelephoneNumber(); phone != "" {
			card.Add(vcard.FieldTelephone, &vcard.Field{
				Value:  phone,
				Params: vcard.Params{vcard.ParamType: {"CELL"}},
			})
		}
		if phone := msgProps.GetPagerTelephoneNumber(); phone != "" {
			card.Add(vcard.FieldTelephone, &vcard.Field{
				Value:  phone,
				Params: vcard.Params{vcard.ParamType: {"PAGER"}},
			})
		}
		if phone := msgProps.GetOtherTelephoneNumber(); phone != "" {
			card.Add(vcard.FieldTelephone, &vcard.Field{
				Value:  phone,
				Params: vcard.Params{vcard.ParamType: {"VOICE"}},
			})
		}
	}

	// Organization and title
	if org := props.GetCompanyName(); org != "" {
		dept := props.GetDepartmentName()
		if dept != "" {
			card.SetValue(vcard.FieldOrganization, org+";"+dept)
		} else {
			card.SetValue(vcard.FieldOrganization, org)
		}
	}
	if title := props.GetTitle(); title != "" {
		card.SetValue(vcard.FieldTitle, title)
	}

	// Work address
	// ADR format: PO Box;Extended;Street;City;Region;Postal Code;Country
	workStreet := props.GetWorkAddressStreet()
	workCity := props.GetWorkAddressCity()
	workState := props.GetWorkAddressState()
	workPostal := props.GetWorkAddressPostalCode()
	workCountry := props.GetWorkAddressCountry()
	if workStreet != "" || workCity != "" || workState != "" || workPostal != "" || workCountry != "" {
		card.Add(vcard.FieldAddress, &vcard.Field{
			Value:  ";;" + workStreet + ";" + workCity + ";" + workState + ";" + workPostal + ";" + workCountry,
			Params: vcard.Params{vcard.ParamType: {"WORK"}},
		})
	}

	// Home address
	homeStreet := props.GetHomeAddressStreet()
	homeCity := props.GetHomeAddressCity()
	homeState := props.GetHomeAddressStateOrProvince()
	homePostal := props.GetHomeAddressPostalCode()
	homeCountry := props.GetHomeAddressCountry()
	if homeStreet != "" || homeCity != "" || homeState != "" || homePostal != "" || homeCountry != "" {
		card.Add(vcard.FieldAddress, &vcard.Field{
			Value:  ";;" + homeStreet + ";" + homeCity + ";" + homeState + ";" + homePostal + ";" + homeCountry,
			Params: vcard.Params{vcard.ParamType: {"HOME"}},
		})
	}

	// Birthday
	if birthday := props.GetBirthdayLocal(); birthday > 0 {
		t := time.Unix(birthday, 0)
		if t.Year() >= 1900 && t.Year() <= 2100 {
			card.SetValue(vcard.FieldBirthday, t.Format("2006-01-02"))
		}
	}

	return card
}
