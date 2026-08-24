package presets

import (
	"sort"
)

// Groups whose folders nest are one game.
//
// groupKey matches on Steam AppID first and normalised name second, which
// misses whenever the same game is found twice under two names. That happens
// constantly: an Unreal title is discovered once by its project codename and
// once by its store name, a repack is found under the AppID while the
// installed copy is found by name, and a manifest entry points at a Save
// subfolder while a directory sweep finds its parent.
//
// The result is two rows for one game, and the numbers say what that costs:
// one is usually named worse and has no AppID, so it appears on the shelf with
// no cover art while its twin, two rows down, has both.
//
// Nesting settles it without guessing. If one row's folder is inside another's
// then they are the same game's data — a save folder inside a save folder is
// not two games, whatever the two rows are called. Nothing here compares
// names, so nothing here can mistake two games with similar titles for one:
//
//	MOUSE                          AppData/LocalLow/Fumi Games/MOUSE
//	MOUSE: P.I. For Hire           AppData/LocalLow/Fumi Games/MOUSE/Save
//
// It resolves cases that looked like they needed a database, too. An Unreal
// game's saves live under its project codename, and the manifest entry for the
// real title points inside that folder — so Sandfall is Clair Obscur:
// Expedition 33 by containment, with no codename mapping anywhere.
//
// Merging is by path only. A shared parent is emphatically not enough:
// everything under Documents/My Games shares one, and treating that as
// sameness merges a user's whole library into a single game.
//
// Two folders nesting is not proof on its own, which a real machine showed:
//
//	Documents/Trackmania            Trackmania (2020),  app 2225070
//	Documents/TrackMania/Profiles   Trackmania United Forever, app 7200
//
// Windows paths are case-insensitive, so those are one directory and the 2008
// game's folders really do sit inside the 2020 game's. Two games, nested, and
// merging them would have offered both as one thing to track — so one sync
// unit covering two games' saves.
//
// Differing Steam AppIDs settle it: each row was matched to a game, they were
// matched to different ones, and that is stronger evidence of difference than
// nesting is of sameness. A set with no AppID still merges, which is the case
// this exists for.
func mergeNestedGroups(saves []DiscoveredSave) {
	if len(saves) < 2 {
		return
	}

	// Sorting by path puts every folder immediately after the folder that
	// contains it, so one pass with a stack of open ancestors finds every
	// nesting — rather than comparing all pairs, which is quadratic in the
	// number of discoveries and a scan can return hundreds.
	order := make([]int, 0, len(saves))
	for i := range saves {
		if cleanForMatch(saves[i].SavePath) != "" {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		return cleanForMatch(saves[order[a]].SavePath) < cleanForMatch(saves[order[b]].SavePath)
	})

	uf := newUnionFind(len(saves))
	// The AppID a merged set has settled on, if any. Two sets that each know
	// their AppID and disagree are two games, and no amount of nesting says
	// otherwise — see mergeRefusedByAppID.
	setApp := map[int]string{}
	for i := range saves {
		if saves[i].AppID != "" {
			setApp[i] = saves[i].AppID
		}
	}

	var stack []int // indices whose folders are still open ancestors
	for _, i := range order {
		for len(stack) > 0 && !pathWithin(saves[i].SavePath, saves[stack[len(stack)-1]].SavePath) {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			a, b := uf.find(i), uf.find(stack[len(stack)-1])
			if a != b {
				appA, appB := setApp[a], setApp[b]
				if appA == "" || appB == "" || appA == appB {
					uf.union(i, stack[len(stack)-1])
					root := uf.find(i)
					if appA != "" {
						setApp[root] = appA
					} else if appB != "" {
						setApp[root] = appB
					}
				}
			}
		}
		stack = append(stack, i)
	}

	// One group id per merged set. The row carrying a Steam AppID names the
	// set: it is the one that resolves to cover art, and the id is what the
	// client asks about.
	best := map[int]int{}
	for _, i := range order {
		root := uf.find(i)
		cur, seen := best[root]
		if !seen || betterGroupRepresentative(saves[i], saves[cur]) {
			best[root] = i
		}
	}
	for _, i := range order {
		rep := best[uf.find(i)]
		if rep == i {
			continue
		}
		saves[i].GroupID = saves[rep].GroupID

		// The group agreed which game this is, so every row in it says so.
		// Without this the merge is invisible where it matters most: tracking
		// is done from one row, and it takes that row's name and AppID — so
		// the outer folder, which is the one offered as the game, would still
		// be tracked as "Sandfall (Epic/Unreal Save)" with no AppID and no
		// cover, while the row that knew it was Clair Obscur sat underneath it.
		if saves[i].AppID == "" && saves[rep].AppID != "" {
			saves[i].AppID = saves[rep].AppID
			// The name travels with the AppID, never on its own. A row with an
			// AppID was matched against the manifest, so its name is the store
			// name rather than a folder name or a project codename.
			if saves[rep].Name != "" {
				saves[i].Name = saves[rep].Name
			}
		}
	}
}

// betterGroupRepresentative reports whether a should name the group instead of
// b.
//
// An AppID wins: it is what a cover lookup needs, and a row that has one is a
// row the manifest recognised. Between two rows that both have one, or neither,
// the outer folder wins — it is the one a user recognises as "the game's save
// folder" rather than a slot inside it.
func betterGroupRepresentative(a, b DiscoveredSave) bool {
	aApp, bApp := a.AppID != "", b.AppID != ""
	if aApp != bApp {
		return aApp
	}
	return pathWithin(b.SavePath, a.SavePath)
}

// unionFind tracks which discoveries have been found to be the same game.
type unionFind struct{ parent []int }

func newUnionFind(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(i int) int {
	for u.parent[i] != i {
		u.parent[i] = u.parent[u.parent[i]] // path halving
		i = u.parent[i]
	}
	return i
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}
