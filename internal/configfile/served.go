package configfile

import "strings"

// servedAccounts is every account conns serve, in their order, with its
// entry where there is one, and the entries no connection serves. The
// file keys an entry by the account's name alone, so it applies to the
// account of that name on whichever forge a connection serves it.
func servedAccounts(conns []Connection, entries []Account) (served, unserved []Account) {
	byName := map[string]*Account{}
	for i := range entries {
		byName[strings.ToLower(entries[i].Name)] = &entries[i]
	}
	used, usedName := map[string]bool{}, map[string]bool{}
	for _, in := range conns {
		for _, name := range in.Accounts {
			key := AccountKey(in.Forge, name)
			if used[key] {
				continue
			}
			used[key], usedName[strings.ToLower(name)] = true, true
			if e, ok := byName[strings.ToLower(name)]; ok {
				a := *e
				a.Forge = in.Forge
				served = append(served, a)
			} else {
				served = append(served, Account{Forge: in.Forge, Name: name})
			}
		}
	}
	for _, e := range entries {
		if !usedName[strings.ToLower(e.Name)] {
			unserved = append(unserved, e)
		}
	}
	return served, unserved
}

// Unserved lists the account entries no connection serves: kept, for when
// one does again, but not run.
func (f *File) Unserved() []Account { return f.unserved }
