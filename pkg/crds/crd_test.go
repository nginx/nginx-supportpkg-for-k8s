package crds

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNICCRDList(t *testing.T) {
	crdList := GetNICCRDList()
	assert.NotEmpty(t, crdList)

	foundSignatures := false
	for _, crd := range crdList {
		if crd.Resource == "apsignatures" && crd.Group == "appprotect.f5.com" {
			foundSignatures = true
			break
		}
	}
	assert.True(t, foundSignatures, "GetNICCRDList should include apsignatures")
}

func TestGetPLMCRDList(t *testing.T) {
	crdList := GetPLMCRDList()
	assert.NotEmpty(t, crdList)

	expectedResources := map[string]string{
		"aplogconfs":   "appprotect.f5.com",
		"appolicies":   "appprotect.f5.com",
		"apusersigs":   "appprotect.f5.com",
		"apsignatures": "appprotect.f5.com",
		"seaweedfses":  "seaweedfs.com",
	}

	for res, group := range expectedResources {
		found := false
		for _, crd := range crdList {
			if crd.Resource == res && crd.Group == group {
				found = true
				break
			}
		}
		assert.True(t, found, "GetPLMCRDList should include "+res)
	}
}
