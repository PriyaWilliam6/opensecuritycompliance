// This file is autogenerated. Modify as per your task needs.

package main

// ValidateAWSAppConnector :
func (inst *TaskInstance) ValidateAWSAppConnector(inputs *UserInputs, outputs *Outputs) (err error) {

	outputs.IsValidated, err = inputs.Validate()

	return err
}