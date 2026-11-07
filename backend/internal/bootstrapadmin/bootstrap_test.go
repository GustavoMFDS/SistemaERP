package bootstrapadmin

import "testing"

func TestValidateAcceptsFreshInstallIdentity(t *testing.T) {
	input := Input{
		LegalName:  "Esterium Laboratorio LTDA",
		TradeName:  "Esterium Lab",
		CNPJ:       "00000000000191",
		AdminName:  "Pessoa Administradora",
		AdminEmail: "admin@esterium.invalid",
	}
	if err := Validate(input, "senha-local-sintetica-forte"); err != nil {
		t.Fatalf("Validate rejected valid setup: %v", err)
	}
}

func TestValidateRejectsUnsafeInputs(t *testing.T) {
	base := Input{
		LegalName:  "Esterium Laboratorio LTDA",
		TradeName:  "Esterium Lab",
		CNPJ:       "00000000000191",
		AdminName:  "Pessoa Administradora",
		AdminEmail: "admin@esterium.invalid",
	}
	tests := []struct {
		name     string
		mutate   func(*Input)
		password string
	}{
		{name: "invalid cnpj checksum", mutate: func(v *Input) { v.CNPJ = "00000000000192" }, password: "senha-local-sintetica-forte"},
		{name: "formatted cnpj", mutate: func(v *Input) { v.CNPJ = "00.000.000/0001-91" }, password: "senha-local-sintetica-forte"},
		{name: "email display name", mutate: func(v *Input) { v.AdminEmail = "Admin <admin@esterium.invalid>" }, password: "senha-local-sintetica-forte"},
		{name: "short password", mutate: func(*Input) {}, password: "curta"},
		{name: "bcrypt truncation", mutate: func(*Input) {}, password: "x1234567890123456789012345678901234567890123456789012345678901234567890123"},
		{name: "newline in name", mutate: func(v *Input) { v.AdminName = "Admin\nOther" }, password: "senha-local-sintetica-forte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := base
			tt.mutate(&input)
			if err := Validate(input, tt.password); err == nil {
				t.Fatal("Validate accepted unsafe input")
			}
		})
	}
}

func TestValidCNPJ(t *testing.T) {
	for _, value := range []string{"00000000000191", "11222333000181"} {
		if !validCNPJ(value) {
			t.Errorf("validCNPJ(%q) = false", value)
		}
	}
	for _, value := range []string{"00000000000000", "11111111111111", "11222333000182"} {
		if validCNPJ(value) {
			t.Errorf("validCNPJ(%q) = true", value)
		}
	}
}
