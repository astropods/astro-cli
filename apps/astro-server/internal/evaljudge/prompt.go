package evaljudge

import "strings"

var systemInstruction = `Predict how a human reviewer will judge whether the trace belongs in this eval dataset. A good verdict means the agent output is good given the trace input. A bad verdict means the agent output is bad given the trace input. An unknown verdict means the trace is irrelevant, ambiguous, or not useful for the dataset.

Return an overall verdict_score from -1 to 1: 1 means very likely good, -1 means very likely bad, and scores near 0 mean unknown. Return confidence from 0 to 100.

Return exactly one score from -1 to 1 for each rubric dimension: ` + criterionDimensionPromptList() + `. Positive values mean the trace satisfies the criterion, negative values mean it violates the criterion, and values near 0 mean it is unclear or not relevant. The overall verdict_score should be consistent with the criterion scores, but is not a simple average; weigh each criterion by its importance to whether the trace is a useful good, bad, or unknown dataset example.

When a next user message is provided, use it as context for how the user reacted to the agent output. Interpret the original message directly rather than relying on a precomputed sentiment label.

Write the explanation as one complete sentence of at most 220 characters. Name the most relevant evidence without inventing missing signals, such as thumbs feedback, the reaction inferred from the next user message, prior examples, rubric criteria, or other trace facts. Do not quote or restate any user or agent message. Return only the structured response requested by the supplied JSON schema.`

func criterionDimensionPromptList() string {
	dimensions := criterionDimensionStrings()
	switch len(dimensions) {
	case 0:
		return ""
	case 1:
		return dimensions[0]
	case 2:
		return strings.Join(dimensions, " and ")
	default:
		return strings.Join(dimensions[:len(dimensions)-1], ", ") + ", and " + dimensions[len(dimensions)-1]
	}
}
