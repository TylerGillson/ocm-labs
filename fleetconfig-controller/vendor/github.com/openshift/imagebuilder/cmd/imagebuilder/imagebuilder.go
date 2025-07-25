package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/distribution/reference"
	dockerregistrytypes "github.com/docker/docker/api/types/registry"
	docker "github.com/fsouza/go-dockerclient"
	"k8s.io/klog"

	"github.com/openshift/imagebuilder"
	"github.com/openshift/imagebuilder/dockerclient"
)

func init() {
	klog.InitFlags(flag.CommandLine)
}

func main() {
	log.SetFlags(0)
	options := dockerclient.NewClientExecutor(nil)
	var tags stringSliceFlag
	var target string
	var dockerfilePath string
	var imageFrom string
	var privileged bool
	var version bool
	var mountSpecs stringSliceFlag

	VERSION := "1.2.16"
	arguments := stringMapFlag{}

	flag.Var(&tags, "t", "The name to assign this image, if any. May be specified multiple times.")
	flag.Var(&tags, "tag", "The name to assign this image, if any. May be specified multiple times.")
	flag.Var(&arguments, "build-arg", "An optional list of build-time variables usable as ARG in Dockerfile. Use --build-arg ARG1=VAL1 --build-arg ARG2=VAL2 syntax for passing multiple build args.")
	flag.StringVar(&dockerfilePath, "f", dockerfilePath, "An optional path to a Dockerfile to use. You may pass multiple docker files using the operating system delimiter.")
	flag.StringVar(&dockerfilePath, "file", dockerfilePath, "An optional path to a Dockerfile to use. You may pass multiple docker files using the operating system delimiter.")
	flag.StringVar(&imageFrom, "from", imageFrom, "An optional FROM to use instead of the one in the Dockerfile.")
	flag.StringVar(&target, "target", "", "The name of a stage within the Dockerfile to build.")
	flag.Var(&mountSpecs, "mount", "An optional list of files and directories to mount during the build. Use SRC:DST syntax for each path.")
	flag.BoolVar(&options.AllowPull, "allow-pull", true, "Pull the images that are not present.")
	flag.BoolVar(&options.IgnoreUnrecognizedInstructions, "ignore-unrecognized-instructions", true, "If an unrecognized Docker instruction is encountered, warn but do not fail the build.")
	flag.BoolVar(&options.StrictVolumeOwnership, "strict-volume-ownership", false, "Due to limitations in docker `cp`, owner permissions on volumes are lost. This flag will fail builds that might fall victim to this.")
	flag.BoolVar(&privileged, "privileged", false, "Builds run as privileged containers instead of restricted containers.")
	flag.BoolVar(&version, "version", false, "Display imagebuilder version.")

	flag.Parse()

	args := flag.Args()
	if version {
		fmt.Println(VERSION)
		return
	}

	if len(args) != 1 {
		log.Fatalf("You must provide one argument, the name of a directory to build")
	}

	options.Directory = args[0]
	if len(tags) > 0 {
		options.Tag = tags[0]
		options.AdditionalTags = tags[1:]
	}
	if len(dockerfilePath) == 0 {
		dockerfilePath = filepath.Join(options.Directory, "Dockerfile")
	}

	if privileged {
		if options.HostConfig == nil {
			options.HostConfig = &docker.HostConfig{}
		}
		options.HostConfig.Privileged = true
	}

	var mounts []dockerclient.Mount
	for _, s := range mountSpecs {
		segments := strings.Split(s, ":")
		if len(segments) != 2 {
			log.Fatalf("--mount must be of the form SOURCE:DEST")
		}
		mounts = append(mounts, dockerclient.Mount{SourcePath: segments[0], DestinationPath: segments[1]})
	}
	options.TransientMounts = mounts

	options.Out, options.ErrOut = os.Stdout, os.Stderr
	authConfigurations, err := docker.NewAuthConfigurationsFromDockerCfg()
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			klog.Warning("No docker configuration found")
		} else {
			log.Fatalf("reading authentication configurations: %v", err)
		}
	}
	if authConfigurations == nil {
		klog.V(4).Info("No authentication secrets found")
	}

	options.AuthFn = func(name string) ([]dockerregistrytypes.AuthConfig, bool) {
		if authConfigurations != nil {
			if authConfig, ok := authConfigurations.Configs[name]; ok {
				klog.V(4).Infof("Found authentication secret for registry %q", name)
				return []dockerregistrytypes.AuthConfig{{
					Username:      authConfig.Username,
					Password:      authConfig.Password,
					Email:         authConfig.Email,
					ServerAddress: authConfig.ServerAddress,
				}}, true
			}
			if named, err := reference.ParseNormalizedNamed(name); err == nil {
				domain := reference.Domain(named)
				if authConfig, ok := authConfigurations.Configs[domain]; ok {
					klog.V(4).Infof("Found authentication secret for registry %q", domain)
					return []dockerregistrytypes.AuthConfig{{
						Username:      authConfig.Username,
						Password:      authConfig.Password,
						Email:         authConfig.Email,
						ServerAddress: authConfig.ServerAddress,
					}}, true
				}
				if domain == "docker.io" || strings.HasSuffix(domain, ".docker.io") {
					var auths []dockerregistrytypes.AuthConfig
					for _, aka := range []string{"docker.io", "index.docker.io", "https://index.docker.io/v1/"} {
						if aka == domain {
							continue
						}
						if authConfig, ok := authConfigurations.Configs[aka]; ok {
							klog.V(4).Infof("Found authentication secret for registry %q", aka)
							auths = append(auths, dockerregistrytypes.AuthConfig{
								Username:      authConfig.Username,
								Password:      authConfig.Password,
								Email:         authConfig.Email,
								ServerAddress: authConfig.ServerAddress,
							})
						}
					}
					if len(auths) > 0 {
						return auths, true
					}
				}
			}
		}
		return nil, false
	}
	options.LogFn = func(format string, args ...interface{}) {
		if klog.V(2) {
			log.Printf("Builder: "+format, args...)
		} else {
			fmt.Fprintf(options.Out, "--> %s\n", fmt.Sprintf(format, args...))
		}
	}

	dockerfiles := filepath.SplitList(dockerfilePath)
	if len(dockerfiles) == 0 {
		dockerfiles = []string{filepath.Join(options.Directory, "Dockerfile")}
	}

	if err := build(dockerfiles[0], dockerfiles[1:], arguments, imageFrom, target, options); err != nil {
		log.Fatal(err.Error())
	}
}

func build(dockerfile string, additionalDockerfiles []string, arguments map[string]string, from string, target string, e *dockerclient.ClientExecutor) error {
	if err := e.DefaultExcludes(); err != nil {
		return fmt.Errorf("error: Could not parse default .dockerignore: %v", err)
	}

	client, err := docker.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("error: No connection to Docker available: %v", err)
	}
	e.Client = client

	// TODO: handle signals
	defer func() {
		for _, err := range e.Release() {
			fmt.Fprintf(e.ErrOut, "error: Unable to clean up build: %v\n", err)
		}
	}()

	node, err := imagebuilder.ParseFile(dockerfile)
	if err != nil {
		return err
	}
	for _, s := range additionalDockerfiles {
		additionalNode, err := imagebuilder.ParseFile(s)
		if err != nil {
			return err
		}
		node.Children = append(node.Children, additionalNode.Children...)
	}

	b := imagebuilder.NewBuilder(arguments)
	stages, err := imagebuilder.NewStages(node, b)
	if err != nil {
		return err
	}
	stages, ok := stages.ByTarget(target)
	if !ok {
		return fmt.Errorf("error: The target %q was not found in the provided Dockerfile", target)
	}

	lastExecutor, err := e.Stages(b, stages, from)
	if err != nil {
		return err
	}

	return lastExecutor.Commit(stages[len(stages)-1].Builder)
}

type stringSliceFlag []string

func (f *stringSliceFlag) Set(s string) error {
	*f = append(*f, s)
	return nil
}

func (f *stringSliceFlag) String() string {
	return strings.Join(*f, " ")
}

type stringMapFlag map[string]string

func (f *stringMapFlag) String() string {
	args := []string{}
	for k, v := range *f {
		args = append(args, strings.Join([]string{k, v}, "="))
	}
	return strings.Join(args, " ")
}

func (f *stringMapFlag) Set(value string) error {
	kv := strings.Split(value, "=")
	(*f)[kv[0]] = kv[1]
	return nil
}
