package kobo

// Device represents a device.
type Device struct {
	ID       string
	Name     string
	Hardware string
}

// Devices.
var (
	DeviceTouchAB               = Device{"00000000-0000-0000-0000-000000000310", "Kobo Touch A/B", "kobo3"}
	DeviceTouchC                = Device{"00000000-0000-0000-0000-000000000320", "Kobo Touch C", "kobo4"}
	DeviceMini                  = Device{"00000000-0000-0000-0000-000000000340", "Kobo Mini", "kobo4"}
	DeviceGlo                   = Device{"00000000-0000-0000-0000-000000000330", "Kobo Glo", "kobo4"}
	DeviceGloHD                 = Device{"00000000-0000-0000-0000-000000000371", "Kobo Glo HD", "kobo6"}
	DeviceTouch2                = Device{"00000000-0000-0000-0000-000000000372", "Kobo Touch 2.0", "kobo6"}
	DeviceAura                  = Device{"00000000-0000-0000-0000-000000000360", "Kobo Aura", "kobo5"}
	DeviceAuraHD                = Device{"00000000-0000-0000-0000-000000000350", "Kobo Aura HD", "kobo4"}
	DeviceAuraH2O               = Device{"00000000-0000-0000-0000-000000000370", "Kobo Aura H2O", "kobo5"}
	DeviceAuraH2OEdition2v1     = Device{"00000000-0000-0000-0000-000000000374", "Kobo Aura H2O Edition 2 v1", "kobo6"}
	DeviceAuraH2OEdition2v2     = Device{"00000000-0000-0000-0000-000000000378", "Kobo Aura H2O Edition 2 v2", "kobo7"}
	DeviceAuraONE               = Device{"00000000-0000-0000-0000-000000000373", "Kobo Aura ONE", "kobo6"}
	DeviceAuraONELimitedEdition = Device{"00000000-0000-0000-0000-000000000381", "Kobo Aura ONE Limited Edition", "kobo6"}
	DeviceAuraEdition2v1        = Device{"00000000-0000-0000-0000-000000000375", "Kobo Aura Edition 2 v1", "kobo6"}
	DeviceAuraEdition2v2        = Device{"00000000-0000-0000-0000-000000000379", "Kobo Aura Edition 2 v2", "kobo7"}
	DeviceClaraHD               = Device{"00000000-0000-0000-0000-000000000376", "Kobo Clara HD", "kobo7"}
	Devices                     = []Device{DeviceTouchAB, DeviceTouchC, DeviceMini, DeviceGlo, DeviceGloHD, DeviceTouch2, DeviceAura, DeviceAuraHD, DeviceAuraH2O, DeviceAuraH2OEdition2v1, DeviceAuraH2OEdition2v2, DeviceAuraONE, DeviceAuraONELimitedEdition, DeviceAuraEdition2v1, DeviceAuraEdition2v2}
)
