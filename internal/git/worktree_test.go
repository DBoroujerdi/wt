package git

import (
	"testing"
)

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"main", "main"},
		{"feature/add-login", "feature_add-login"},
		{"feat/XXX-example-branch", "feat_XXX-example-branch"},
		{"release:v1.0", "release_v1.0"},
		{"branch with spaces", "branch_with_spaces"},
		{"path\\with\\backslashes", "path_with_backslashes"},
		{"branch*with*stars", "branch_with_stars"},
		{"branch?with?questions", "branch_with_questions"},
		{"branch<with>angles", "branch_with_angles"},
		{"branch|with|pipes", "branch_with_pipes"},
		{"branch\"with\"quotes", "branch_with_quotes"},
		{"complex/branch:name with*special?chars", "complex_branch_name_with_special_chars"},
		{"simple-branch-name", "simple-branch-name"},
		{"", ""},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := SanitizeBranchName(test.input)
			if result != test.expected {
				t.Errorf("SanitizeBranchName(%q) = %q, want %q", test.input, result, test.expected)
			}
		})
	}
}
