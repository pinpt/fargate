package ecs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	awsecs "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/jpignata/fargate/console"
)

const logStreamPrefix = "fargate"

var taskDefinitionCache = make(map[string]*awsecs.TaskDefinition)

type CreateTaskDefinitionInput struct {
	Cpu              string
	EnvVars          []EnvVar
	ExecutionRoleArn string
	Image            string
	Memory           string
	Name             string
	Port             int64
	LogGroupName     string
	LogRegion        string
	TaskRole         string
	Type             string
	Compatibility    string
	GPU              int64
}

type KeyValue struct {
	Key   string
	Value string
}

type EnvVar KeyValue

type Tag KeyValue

func (ecs *ECS) CreateTaskDefinition(input *CreateTaskDefinitionInput) string {
	console.Debug("Creating ECS task definition")

	logConfiguration := &awsecs.LogConfiguration{
		LogDriver: aws.String(awsecs.LogDriverAwslogs),
		Options: map[string]*string{
			"awslogs-region":        aws.String(input.LogRegion),
			"awslogs-group":         aws.String(input.LogGroupName),
			"awslogs-stream-prefix": aws.String(logStreamPrefix),
		},
	}

	containerDefinition := &awsecs.ContainerDefinition{
		Environment:      input.Environment(),
		Essential:        aws.Bool(true),
		Image:            aws.String(input.Image),
		LogConfiguration: logConfiguration,
		Name:             aws.String(input.Name),
	}

	if input.Port != 0 {
		containerDefinition.SetPortMappings(
			[]*awsecs.PortMapping{
				&awsecs.PortMapping{
					ContainerPort: aws.Int64(int64(input.Port)),
				},
			},
		)
	}

	if input.GPU != 0 {
		containerDefinition.SetResourceRequirements(
			[]*awsecs.ResourceRequirement{
				{
					Type:  aws.String("GPU"),
					Value: aws.String(strconv.FormatInt(int64(input.GPU), 10)),
				},
			},
		)
	}

	resp, err := ecs.svc.RegisterTaskDefinition(
		&awsecs.RegisterTaskDefinitionInput{
			ContainerDefinitions:    []*awsecs.ContainerDefinition{containerDefinition},
			Cpu:                     aws.String(input.Cpu),
			ExecutionRoleArn:        aws.String(input.ExecutionRoleArn),
			Family:                  aws.String(fmt.Sprintf("%s_%s", input.Type, input.Name)),
			Memory:                  aws.String(input.Memory),
			NetworkMode:             aws.String(awsecs.NetworkModeAwsvpc),
			RequiresCompatibilities: aws.StringSlice([]string{input.Compatibility}),
			TaskRoleArn:             aws.String(input.TaskRole),
		},
	)

	if err != nil {
		console.ErrorExit(err, "Couldn't register ECS task definition")
	}

	td := resp.TaskDefinition

	console.Debug("Created ECS task definition [%s:%d]", aws.StringValue(td.Family), aws.Int64Value(td.Revision))

	return aws.StringValue(td.TaskDefinitionArn)
}

func (input *CreateTaskDefinitionInput) Environment() []*awsecs.KeyValuePair {
	var environment []*awsecs.KeyValuePair

	for _, envVar := range input.EnvVars {
		environment = append(environment,
			&awsecs.KeyValuePair{
				Name:  aws.String(envVar.Key),
				Value: aws.String(envVar.Value),
			},
		)
	}

	return environment
}

func (ecs *ECS) DescribeTaskDefinition(taskDefinitionArn string) *awsecs.TaskDefinition {
	if taskDefinitionCache[taskDefinitionArn] != nil {
		return taskDefinitionCache[taskDefinitionArn]
	}

	resp, err := ecs.svc.DescribeTaskDefinition(
		&awsecs.DescribeTaskDefinitionInput{
			TaskDefinition: aws.String(taskDefinitionArn),
		},
	)

	if err != nil {
		console.ErrorExit(err, "Could not describe ECS task definition")
	}

	taskDefinitionCache[taskDefinitionArn] = resp.TaskDefinition

	return taskDefinitionCache[taskDefinitionArn]
}

func (ecs *ECS) UpdateTaskDefinitionImage(taskDefinitionArn, image string) string {
	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)
	taskDefinition.ContainerDefinitions[0].Image = aws.String(image)

	resp, err := ecs.svc.RegisterTaskDefinition(
		&awsecs.RegisterTaskDefinitionInput{
			ContainerDefinitions:    taskDefinition.ContainerDefinitions,
			Cpu:                     taskDefinition.Cpu,
			ExecutionRoleArn:        taskDefinition.ExecutionRoleArn,
			Family:                  taskDefinition.Family,
			Memory:                  taskDefinition.Memory,
			NetworkMode:             taskDefinition.NetworkMode,
			RequiresCompatibilities: taskDefinition.RequiresCompatibilities,
			TaskRoleArn:             taskDefinition.TaskRoleArn,
			Volumes:                 taskDefinition.Volumes,
		},
	)

	if err != nil {
		console.ErrorExit(err, "Could not register ECS task definition")
	}

	return aws.StringValue(resp.TaskDefinition.TaskDefinitionArn)
}

func (ecs *ECS) AddEnvVarsToTaskDefinition(taskDefinitionArn string, envVars []EnvVar) string {
	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)

	for _, envVar := range envVars {
		keyValuePair := &awsecs.KeyValuePair{
			Name:  aws.String(envVar.Key),
			Value: aws.String(envVar.Value),
		}

		taskDefinition.ContainerDefinitions[0].Environment = append(
			taskDefinition.ContainerDefinitions[0].Environment,
			keyValuePair,
		)
	}

	resp, err := ecs.svc.RegisterTaskDefinition(
		&awsecs.RegisterTaskDefinitionInput{
			ContainerDefinitions:    taskDefinition.ContainerDefinitions,
			Cpu:                     taskDefinition.Cpu,
			ExecutionRoleArn:        taskDefinition.ExecutionRoleArn,
			Family:                  taskDefinition.Family,
			Memory:                  taskDefinition.Memory,
			NetworkMode:             taskDefinition.NetworkMode,
			RequiresCompatibilities: taskDefinition.RequiresCompatibilities,
			TaskRoleArn:             taskDefinition.TaskRoleArn,
			Volumes:                 taskDefinition.Volumes,
		},
	)

	if err != nil {
		console.ErrorExit(err, "Could not register ECS task definition")
	}

	return aws.StringValue(resp.TaskDefinition.TaskDefinitionArn)
}

func (ecs *ECS) AddSecretVarsToTaskDefinition(taskDefinitionArn string, envVars []EnvVar) string {
	var newEnvironment []*awsecs.Secret
	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)
	environment := taskDefinition.ContainerDefinitions[0].Secrets

	for _, envVar := range envVars {
		keyValuePair := &awsecs.Secret{
			Name:      aws.String(envVar.Key),
			ValueFrom: aws.String(envVar.Value),
		}

		newEnvironment = append(newEnvironment, keyValuePair)
	}

	for _, key := range environment {
		found := false
		for _, envKey := range newEnvironment {
			if aws.StringValue(key.Name) == aws.StringValue(envKey.Name) {
				found = true
				break
			}
		}

		if !found {
			newEnvironment = append(newEnvironment, key)
		}
	}

	taskDefinition.ContainerDefinitions[0].Secrets = newEnvironment
	resp, err := ecs.svc.RegisterTaskDefinition(
		&awsecs.RegisterTaskDefinitionInput{
			ContainerDefinitions:    taskDefinition.ContainerDefinitions,
			Cpu:                     taskDefinition.Cpu,
			ExecutionRoleArn:        taskDefinition.ExecutionRoleArn,
			Family:                  taskDefinition.Family,
			Memory:                  taskDefinition.Memory,
			NetworkMode:             taskDefinition.NetworkMode,
			RequiresCompatibilities: taskDefinition.RequiresCompatibilities,
			TaskRoleArn:             taskDefinition.TaskRoleArn,
			Volumes:                 taskDefinition.Volumes,
		},
	)

	if err != nil {
		console.ErrorExit(err, "Could not register ECS task definition")
	}

	return aws.StringValue(resp.TaskDefinition.TaskDefinitionArn)
}

func (ecs *ECS) RemoveEnvVarsFromTaskDefinition(taskDefinitionArn string, keys []string) string {
	var newEnvironment []*awsecs.KeyValuePair

	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)
	environment := taskDefinition.ContainerDefinitions[0].Environment

	for _, keyValuePair := range environment {
		found := false
		for _, key := range keys {
			if aws.StringValue(keyValuePair.Name) == key {
				found = true
				break
			}
		}

		if !found {
			newEnvironment = append(newEnvironment, keyValuePair)
		}
	}

	taskDefinition.ContainerDefinitions[0].Environment = newEnvironment

	resp, err := ecs.svc.RegisterTaskDefinition(
		&awsecs.RegisterTaskDefinitionInput{
			ContainerDefinitions:    taskDefinition.ContainerDefinitions,
			Cpu:                     taskDefinition.Cpu,
			ExecutionRoleArn:        taskDefinition.ExecutionRoleArn,
			Family:                  taskDefinition.Family,
			Memory:                  taskDefinition.Memory,
			NetworkMode:             taskDefinition.NetworkMode,
			RequiresCompatibilities: taskDefinition.RequiresCompatibilities,
			TaskRoleArn:             taskDefinition.TaskRoleArn,
			Volumes:                 taskDefinition.Volumes,
		},
	)

	if err != nil {
		console.ErrorExit(err, "Could not register ECS task definition")
	}

	return aws.StringValue(resp.TaskDefinition.TaskDefinitionArn)
}

func (ecs *ECS) RemoveSecretVarsFromTaskDefinition(taskDefinitionArn string, keys []string) string {
	var newEnvironment []*awsecs.Secret

	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)
	environment := taskDefinition.ContainerDefinitions[0].Secrets

	for _, keyValuePair := range environment {
		found := false
		for _, key := range keys {
			if aws.StringValue(keyValuePair.Name) == key {
				found = true
				break
			}
		}

		if !found {
			newEnvironment = append(newEnvironment, keyValuePair)
		}
	}

	taskDefinition.ContainerDefinitions[0].Secrets = newEnvironment

	resp, err := ecs.svc.RegisterTaskDefinition(
		&awsecs.RegisterTaskDefinitionInput{
			ContainerDefinitions:    taskDefinition.ContainerDefinitions,
			Cpu:                     taskDefinition.Cpu,
			ExecutionRoleArn:        taskDefinition.ExecutionRoleArn,
			Family:                  taskDefinition.Family,
			Memory:                  taskDefinition.Memory,
			NetworkMode:             taskDefinition.NetworkMode,
			RequiresCompatibilities: taskDefinition.RequiresCompatibilities,
			TaskRoleArn:             taskDefinition.TaskRoleArn,
			Volumes:                 taskDefinition.Volumes,
		},
	)

	if err != nil {
		console.ErrorExit(err, "Could not register ECS task definition")
	}

	return aws.StringValue(resp.TaskDefinition.TaskDefinitionArn)
}

func (ecs *ECS) GetEnvVarsFromTaskDefinition(taskDefinitionArn string) []EnvVar {
	var envVars []EnvVar

	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)

	for _, keyValuePair := range taskDefinition.ContainerDefinitions[0].Environment {
		envVars = append(envVars,
			EnvVar{
				Key:   aws.StringValue(keyValuePair.Name),
				Value: aws.StringValue(keyValuePair.Value),
			},
		)
	}

	return envVars
}

func (ecs *ECS) GetSecretVarsFromTaskDefinition(taskDefinitionArn string) []EnvVar {
	var envVars []EnvVar

	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)

	for _, keyValuePair := range taskDefinition.ContainerDefinitions[0].Secrets {
		envVars = append(envVars,
			EnvVar{
				Key:   aws.StringValue(keyValuePair.Name),
				Value: aws.StringValue(keyValuePair.ValueFrom),
			},
		)
	}

	return envVars
}

func (ecs *ECS) UpdateTaskDefinitionCpuAndMemory(taskDefinitionArn, cpu, memory string) string {
	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)

	if cpu != "" {
		taskDefinition.Cpu = aws.String(cpu)
	}

	if memory != "" {
		taskDefinition.Memory = aws.String(memory)
	}

	resp, err := ecs.svc.RegisterTaskDefinition(
		&awsecs.RegisterTaskDefinitionInput{
			ContainerDefinitions:    taskDefinition.ContainerDefinitions,
			Cpu:                     taskDefinition.Cpu,
			ExecutionRoleArn:        taskDefinition.ExecutionRoleArn,
			Family:                  taskDefinition.Family,
			Memory:                  taskDefinition.Memory,
			NetworkMode:             taskDefinition.NetworkMode,
			RequiresCompatibilities: taskDefinition.RequiresCompatibilities,
			TaskRoleArn:             taskDefinition.TaskRoleArn,
			Volumes:                 taskDefinition.Volumes,
		},
	)

	if err != nil {
		console.ErrorExit(err, "Could not register ECS task definition")
	}

	return aws.StringValue(resp.TaskDefinition.TaskDefinitionArn)
}

func (ecs *ECS) getDeploymentId(taskDefinitionArn string) string {
	contents := strings.Split(taskDefinitionArn, ":")
	return contents[len(contents)-1]
}

func (ecs *ECS) GetCpuAndMemoryFromTaskDefinition(taskDefinitionArn string) (string, string) {
	taskDefinition := ecs.DescribeTaskDefinition(taskDefinitionArn)

	return aws.StringValue(taskDefinition.Cpu), aws.StringValue(taskDefinition.Memory)
}
