package proj

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    "emergion-sovereign-runtime/internal/core"
)

func TestOperatorNavigationProjectsExistingSectionsWithoutAuthorityMutation(t *testing.T) {
    st := core.EmptyState()
    st.Events = 3
    st.TipHash = "operator-navigation-tip"
    before := st.TipHash
    root := t.TempDir()
    receipt, err := Current(root, st)
    if err != nil {
        t.Fatal(err)
    }
    htmlBytes, err := os.ReadFile(filepath.Join(root, "field.html"))
    if err != nil {
        t.Fatal(err)
    }
    html := string(htmlBytes)
    if !strings.Contains(html, "Operator navigation") {
        t.Fatal("operator navigation drawer missing")
    }
    for _, section := range []string{
        "realization-field", "human-final", "verified-deliveries",
        "lifecycle-projection", "spatial-convergence",
        "spatial-emergion", "saw-work",
    } {
        if !strings.Contains(html, `href="#`+section+`"`) {
            t.Errorf("drawer link to %s missing", section)
        }
        if !strings.Contains(html, `id="`+section+`"`) {
            t.Errorf("drawer target %s missing", section)
        }
    }
    if st.TipHash != before || receipt.TipHash != before {
        t.Fatal("operator navigation changed canonical FIELD tip")
    }
    if !strings.Contains(html, "governance remains unchanged") {
        t.Fatal("operator navigation must disclose its non-authoritative role")
    }
}
