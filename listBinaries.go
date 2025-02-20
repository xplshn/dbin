package main

func listBinaries(uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	var allBinaries []binaryEntry

	for _, bin := range uRepoIndex {
		name, pkgId, version, description, rank := bin.Name, bin.PkgId, bin.Version, bin.Description, bin.Rank

		if name != "" {
			allBinaries = append(allBinaries, binaryEntry{
				Name:        name,
				PkgId:       pkgId,
				Version:     version,
				Description: description,
				Rank:        rank,
			})
		}
	}

	return allBinaries, nil
}
