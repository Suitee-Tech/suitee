package handlers

import (
	"encoding/json"
	"testing"
)

// O binding do POST precisa carregar alíquota e base: sem os campos na request,
// o desconto por percentual era gravado sem a memória de cálculo (o domínio
// valida, mas nunca recebia os valores).
func TestAddDeductionRequest_BindsRateAndBase(t *testing.T) {
	body := `{"kind":"INSS","amount":5760,"rate_pct":20,"base_cents":2880000}`
	var req AddDeductionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.RatePct != 20 || req.BaseCents != 2880000 {
		t.Fatalf("alíquota/base não vinculadas: rate=%v base=%v", req.RatePct, req.BaseCents)
	}
	if req.Amount != 5760 || req.Kind != "INSS" {
		t.Fatalf("campos básicos perdidos: %+v", req)
	}
}
