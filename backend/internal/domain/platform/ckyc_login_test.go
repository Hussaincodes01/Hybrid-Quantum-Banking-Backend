package platform

import "testing"

// CKYC is the app's login handle, so these tests pin its behaviour: seeded
// accounts must be reachable by number, the format must be enforced, and a
// wrong number must not reveal whether it exists.

func seededService(t *testing.T) *Service {
	t.Helper()
	svc := NewService()
	t.Cleanup(svc.Close)
	if svc.NeedsSeeding() {
		if err := svc.SeedDemoUsers(); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return svc
}

func TestSeedUsersHaveUniqueTenDigitCKYC(t *testing.T) {
	svc := seededService(t)

	seen := map[string]string{}
	for id, u := range svc.users {
		if len(u.CKYC) != 10 {
			t.Errorf("user %s (%s) has CKYC %q, want 10 digits", u.Name, id, u.CKYC)
			continue
		}
		for _, r := range u.CKYC {
			if r < '0' || r > '9' {
				t.Errorf("user %s has non-digit CKYC %q", u.Name, u.CKYC)
				break
			}
		}
		if prev, dup := seen[u.CKYC]; dup {
			t.Errorf("CKYC %s assigned to both %s and %s", u.CKYC, prev, u.Name)
		}
		seen[u.CKYC] = u.Name

		if svc.ckycIndex[u.CKYC] != id {
			t.Errorf("ckycIndex[%s] = %q, want %q", u.CKYC, svc.ckycIndex[u.CKYC], id)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no seeded users carried a CKYC number")
	}
}

func TestLoginWithCKYC(t *testing.T) {
	svc := seededService(t)

	// Jiyad is the primary demo identity.
	const ckyc = "2000000001"
	userID, ok := svc.ckycIndex[ckyc]
	if !ok {
		t.Fatalf("CKYC %s not seeded", ckyc)
	}

	res, err := svc.LoginWithPIN(PinLoginRequest{
		CKYC:                ckyc,
		PIN:                 "123456",
		DeviceIDFingerprint: "device-demo-fp",
	})
	if err != nil {
		t.Fatalf("login by CKYC failed: %v", err)
	}
	if res.UserID != userID {
		t.Errorf("resolved userId %q, want %q", res.UserID, userID)
	}
	if res.AccessToken == "" {
		t.Error("expected an access token")
	}
	if res.CKYC != ckyc {
		t.Errorf("response CKYC = %q, want %q", res.CKYC, ckyc)
	}
}

func TestLoginWithCKYCRejections(t *testing.T) {
	svc := seededService(t)
	base := func(mut func(*PinLoginRequest)) PinLoginRequest {
		r := PinLoginRequest{CKYC: "2000000001", PIN: "123456", DeviceIDFingerprint: "device-demo-fp"}
		mut(&r)
		return r
	}

	t.Run("wrong PIN", func(t *testing.T) {
		if _, err := svc.LoginWithPIN(base(func(r *PinLoginRequest) { r.PIN = "000000" })); err == nil {
			t.Fatal("expected rejection for a wrong PIN")
		}
	})

	t.Run("unknown CKYC is indistinguishable from a wrong PIN", func(t *testing.T) {
		_, err := svc.LoginWithPIN(base(func(r *PinLoginRequest) { r.CKYC = "9999999999" }))
		if err == nil {
			t.Fatal("expected rejection for an unknown CKYC")
		}
		// Must not confirm the number does not exist — that would let an
		// attacker enumerate valid CKYC numbers.
		if got := err.Error(); got == "user not found" || got == "ckyc not found" {
			t.Errorf("error %q leaks account existence", got)
		}
	})

	t.Run("bad format", func(t *testing.T) {
		for _, bad := range []string{"12345", "20000000011", "20000000a1"} {
			if _, err := svc.LoginWithPIN(base(func(r *PinLoginRequest) { r.CKYC = bad })); err == nil {
				t.Errorf("CKYC %q should be rejected as malformed", bad)
			}
		}
	})

	t.Run("no identifier at all", func(t *testing.T) {
		_, err := svc.LoginWithPIN(PinLoginRequest{PIN: "123456", DeviceIDFingerprint: "d"})
		if err == nil {
			t.Fatal("expected rejection when no identifier is supplied")
		}
	})
}

func TestRegisteredUserGetsCKYCOutsideSeedRange(t *testing.T) {
	svc := seededService(t)

	res, err := svc.Register(RegisterRequest{
		Name:                "New Person",
		Mobile:              "9812345678",
		DeviceIDFingerprint: "fp-new-device",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	user, ok := svc.users[res.UserID]
	if !ok {
		t.Fatal("registered user missing")
	}
	if len(user.CKYC) != 10 {
		t.Fatalf("new user CKYC %q, want 10 digits", user.CKYC)
	}
	// Runtime allocations start above the seeded block so they cannot collide.
	if user.CKYC < "3000000001" {
		t.Errorf("new user CKYC %s should be allocated above the seed range", user.CKYC)
	}
	if svc.ckycIndex[user.CKYC] != res.UserID {
		t.Error("new user was not added to the CKYC index")
	}
}
