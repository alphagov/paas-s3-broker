package policy

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type PolicyDocument struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

func BuildPolicy(maybeExistingPolicy string, statement Statement) (PolicyDocument, error) {
	if maybeExistingPolicy == "" {
		return PolicyDocument{
			Version:   "2012-10-17",
			Statement: []Statement{statement},
		}, nil
	}

	existingPolicy := PolicyDocument{}
	err := json.Unmarshal([]byte(maybeExistingPolicy), &existingPolicy)
	if err != nil {
		return PolicyDocument{}, err
	}
	if reflect.DeepEqual(existingPolicy, PolicyDocument{}) {
		return PolicyDocument{}, fmt.Errorf(
			"provided json was well-formed, but did not unmarshal into a Policy. Provided JSON: %s",
			maybeExistingPolicy)
	}

	existingPolicy.Statement = append(existingPolicy.Statement, statement)
	return existingPolicy, nil
}

func RemoveUserFromPolicy(existingPolicy string, userArnSuffix string) (PolicyDocument, error) {
	policyDoc := PolicyDocument{}
	err := json.Unmarshal([]byte(existingPolicy), &policyDoc)
	if err != nil {
		return PolicyDocument{}, err
	}
	if reflect.DeepEqual(policyDoc, PolicyDocument{}) {
		return PolicyDocument{}, fmt.Errorf(
			"provided json was well-formed, but did not unmarshal into a Policy. Provided JSON: %s",
			existingPolicy)
	}

	var maintainedStatements []Statement
	for _, stmt := range policyDoc.Statement {
		if !strings.HasSuffix(stmt.Principal.AWS, userArnSuffix) {
			maintainedStatements = append(maintainedStatements, stmt)
		}
	}

	if len(maintainedStatements) == len(policyDoc.Statement) {
		return PolicyDocument{}, fmt.Errorf("could not find a policy statement for user %s", userArnSuffix)
	}

	policyDoc.Statement = maintainedStatements

	return policyDoc, nil
}
