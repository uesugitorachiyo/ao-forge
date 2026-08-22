package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV014ReleaseMetadataIsSourceOwned(t *testing.T) {
	root := repoRoot(t)
	notes, err := os.ReadFile(filepath.Join(root, "docs", "release", "V0.1.4-RELEASE-NOTES.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# AO Forge v0.1.4 Release Notes",
		"governed issue repair",
		"immutable native candidates",
		"Linux x86_64",
		"macOS aarch64",
		"Windows x86_64",
	} {
		if !strings.Contains(string(notes), want) {
			t.Fatalf("v0.1.4 release notes missing %q", want)
		}
	}
}

func TestReleasePublishPromotesExactRehearsedCandidates(t *testing.T) {
	workflow := readPublicationContractFile(t, ".github", "workflows", "release-publish.yml")
	for _, want := range []string{
		"source_commit:",
		"expected_plan_digest:",
		"exact_confirmation:",
		"ao-forge-release-rehearsal-plan-${SOURCE_COMMIT}",
		"ao-forge-release-candidate-*-${SOURCE_COMMIT}",
		"python3 scripts/verify-release-rehearsal.py assemble",
		"cmp --silent",
		"candidate archive digest mismatch",
		"docs/release/V${VERSION}-RELEASE-NOTES.md",
		"--draft",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("release publish workflow missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"GOOS=linux GOARCH=amd64 go build",
		"GOOS=darwin GOARCH=arm64 go build",
		"GOOS=windows GOARCH=amd64 go build",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release publish workflow rebuilds candidate with %q", forbidden)
		}
	}
}

func TestReleaseTagProducerAndFinalizerAreGoverned(t *testing.T) {
	producer := readPublicationContractFile(t, "scripts", "release-tag-producer.sh")
	for _, want := range []string{
		"--dry-run",
		"--live",
		"verify-release-rehearsal.py verify-plan",
		"git tag -s",
		"git push --atomic origin",
		"exact confirmation mismatch",
	} {
		if !strings.Contains(producer, want) {
			t.Fatalf("release tag producer missing %q", want)
		}
	}

	finalizer := readPublicationContractFile(t, ".github", "workflows", "release-finalize.yml")
	for _, want := range []string{
		"workflow_dispatch:",
		"source_commit:",
		"expected_plan_digest:",
		"exact_confirmation:",
		"environment: production-release",
		"release must be an exact draft",
		"release asset inventory mismatch",
		"--method PATCH",
		"-F draft=false",
		"published release readback mismatch",
	} {
		if !strings.Contains(finalizer, want) {
			t.Fatalf("release finalizer missing %q", want)
		}
	}
	for _, forbidden := range []string{"gh release create", "gh release upload", "git tag", "git push"} {
		if strings.Contains(finalizer, forbidden) {
			t.Fatalf("release finalizer contains forbidden mutation %q", forbidden)
		}
	}
}

func TestReleaseFinalizerResolvesDraftWithAuthenticatedList(t *testing.T) {

	finalizer := readPublicationContractFile(t, ".github", "workflows", "release-finalize.yml")
	for _, want := range []string{
		"workflow_source_commit:",
		"WORKFLOW_SOURCE_COMMIT: ${{ inputs.workflow_source_commit }}",
		"releases?per_page=100",
		"matching = [release for release in releases",
		"len(matching) != 1",
		"release = matching[0]",
		"git merge-base --is-ancestor \"$SOURCE_COMMIT\" \"$GITHUB_SHA\"",
		"workflow source must descend from release source",
		"releases-pages.json",
		`Path("releases-pages.json").read_bytes()`,
	} {
		if !strings.Contains(finalizer, want) {
			t.Fatalf("release finalizer missing draft-safe contract %q", want)
		}
	}
	for _, forbidden := range []string{"RELEASES_JSON", "releases_json=$("} {
		if strings.Contains(finalizer, forbidden) {
			t.Fatalf("release finalizer uses oversized environment transfer %q", forbidden)
		}
	}
}

func TestPublicReleaseVerifiersUsePackedBinaryNames(t *testing.T) {
	verify := readPublicationContractFile(t, ".github", "workflows", "release-verify.yml")
	for _, want := range []string{
		"ao-forge-release-smoke/forge",
		"linux_x86_64_smoke=passed",
		"darwin_arm64_smoke=passed",
	} {
		if !strings.Contains(verify, want) {
			t.Fatalf("release verifier missing packed binary contract %q", want)
		}
	}
	if strings.Contains(verify, "ao-forge-release-smoke/ao-forge") {
		t.Fatal("release verifier expects a binary name not present in public archives")
	}

	install := readPublicationContractFile(t, ".github", "workflows", "release-install-verify.yml")
	for _, want := range []string{
		"${extract}/linux/forge",
		"(extract / \"forge.exe\").is_file()",
		"windows_archive_contains=forge.exe",
	} {
		if !strings.Contains(install, want) {
			t.Fatalf("release install verifier missing packed binary contract %q", want)
		}
	}
	for _, forbidden := range []string{"${extract}/linux/ao-forge", "(extract / \"ao-forge.exe\").is_file()"} {
		if strings.Contains(install, forbidden) {
			t.Fatalf("release install verifier expects a binary name not present in public archives: %q", forbidden)
		}
	}
}

func readPublicationContractFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
