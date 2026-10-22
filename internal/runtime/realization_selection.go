package runtime

type RealizationSelection struct {
	TargetThing string
	Selected    string
	RunnersUp   []string
}

func SelectStrongestRealization(p RealizationPopulation) (RealizationSelection, bool, error) {
	order, err := DeriveRealizationOrder(p)
	if err != nil {
		return RealizationSelection{}, false, err
	}

	strongest := order.Strongest()
	if len(strongest) != 1 {
		return RealizationSelection{
			TargetThing: p.TargetThing,
		}, false, nil
	}

	runnersUp := []string{}
	for _, front := range order.Fronts[1:] {
		runnersUp = append(runnersUp, front...)
	}

	return RealizationSelection{
		TargetThing: p.TargetThing,
		Selected:    strongest[0],
		RunnersUp:   runnersUp,
	}, true, nil
}
