package ui

import (
	"testing"

	"pw/internal/project"
	"pw/internal/state"
)

// paths extracts the Path field of each project for order comparison.
func paths(ps []project.Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Path
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestProjectLessDeterministicTiebreak verifies the Path tiebreaker makes
// ordering total: two projects with the same lowercase Name are ordered by
// their exact Path, never left in arbitrary input order.
func TestProjectLessDeterministicTiebreak(t *testing.T) {
	a := project.Project{Name: "App", Path: "/b/app"}
	b := project.Project{Name: "app", Path: "/a/app"}

	// Same lowercase name -> must fall back to Path comparison, and be a
	// strict, antisymmetric order.
	if projectLess(a, b) == projectLess(b, a) {
		t.Fatalf("projectLess not antisymmetric for equal names: less(a,b)=%v less(b,a)=%v",
			projectLess(a, b), projectLess(b, a))
	}
	// "/a/app" < "/b/app" so b should sort before a.
	if !projectLess(b, a) {
		t.Fatalf("expected /a/app to sort before /b/app")
	}
	if projectLess(a, b) {
		t.Fatalf("expected /b/app NOT to sort before /a/app")
	}
}

// TestFavoritesSortedStableAcrossMapPermutations builds many stores whose
// favorite entries collide on lowercase Name and asserts favoritesSorted()
// returns the exact same order regardless of Go's randomized map iteration.
func TestFavoritesSortedStableAcrossMapPermutations(t *testing.T) {
	// Distinct paths, several sharing a lowercase Name ("app", "web"),
	// and a case-only difference ("App" vs "app").
	all := []project.Project{
		{Name: "App", Path: "/one/App"},
		{Name: "app", Path: "/two/app"},
		{Name: "app", Path: "/three/app"},
		{Name: "web", Path: "/x/web"},
		{Name: "Web", Path: "/y/Web"},
		{Name: "zeta", Path: "/z/zeta"},
	}

	favSet := map[string]bool{}
	for _, p := range all {
		favSet[p.Path] = true
	}

	var want []string
	// Run many iterations; Go randomizes map order per range, so repeated
	// runs exercise different input permutations.
	for i := 0; i < 200; i++ {
		m := Model{
			all:   all,
			store: &state.Store{Favorites: favSet, Recent: map[string]int64{}},
		}
		got := paths(m.favoritesSorted())
		if want == nil {
			want = got
			continue
		}
		if !eq(want, got) {
			t.Fatalf("favoritesSorted order not stable across map permutations\n want %v\n got  %v", want, got)
		}
	}
}

// TestSortedProjectsFavoritesPinnedStable asserts favorites pin to the top in
// a deterministic order even with name collisions and case-only differences,
// across repeated builds (which re-run the unstable-input map iteration).
func TestSortedProjectsFavoritesPinnedStable(t *testing.T) {
	all := []project.Project{
		{Name: "beta", Path: "/p/beta"},
		{Name: "App", Path: "/p/App"},
		{Name: "app", Path: "/p/app"},
		{Name: "gamma", Path: "/p/gamma"},
		{Name: "alpha", Path: "/p/alpha"},
	}
	// Favorite two colliding-name projects + one unique.
	favSet := map[string]bool{
		"/p/App": true,
		"/p/app": true,
		"/p/alpha": true,
	}

	var want []string
	for i := 0; i < 200; i++ {
		m := Model{
			all:   all,
			store: &state.Store{Favorites: favSet, Recent: map[string]int64{}},
		}
		got := paths(m.sortedProjects(""))
		if want == nil {
			want = got
			// Favorites must occupy the first 3 slots.
			if len(got) != len(all) {
				t.Fatalf("expected %d projects, got %d", len(all), len(got))
			}
			for _, fp := range got[:3] {
				if !favSet[fp] {
					t.Fatalf("non-favorite %q found in pinned top region %v", fp, got[:3])
				}
			}
			continue
		}
		if !eq(want, got) {
			t.Fatalf("sortedProjects order not stable\n want %v\n got  %v", want, got)
		}
	}
}
