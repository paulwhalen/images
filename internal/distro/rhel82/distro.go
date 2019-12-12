package rhel82

import (
	"errors"
	"sort"
	"strconv"

	"github.com/google/uuid"

	"github.com/osbuild/osbuild-composer/internal/blueprint"
	"github.com/osbuild/osbuild-composer/internal/crypt"
	"github.com/osbuild/osbuild-composer/internal/pipeline"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
)

type RHEL82 struct {
	arches  map[string]arch
	outputs map[string]output
}

type arch struct {
	Name               string
	BootloaderPackages []string
	BuildPackages      []string
}

type output struct {
	Name             string
	MimeType         string
	Packages         []string
	ExcludedPackages []string
	EnabledServices  []string
	DisabledServices []string
	Bootable         bool
	DefaultTarget    string
	KernelOptions    string
	Assembler        *pipeline.Assembler
}

func New() *RHEL82 {
	const GigaByte = 1024 * 1024 * 1024

	r := RHEL82{
		arches:  map[string]arch{},
		outputs: map[string]output{},
	}

	r.arches["x86_64"] = arch{
		Name: "x86_64",
		BootloaderPackages: []string{
			"grub2-pc",
		},
		BuildPackages: []string{
			"grub2-pc",
		},
	}

	r.outputs["ami"] = output{
		Name:     "image.raw.xz",
		MimeType: "application/octet-stream",
		Packages: []string{
			"checkpolicy",
			"chrony",
			"cloud-init",
			"cloud-init",
			"cloud-utils-growpart",
			"@core",
			"dhcp-client",
			"dracut-config-generic",
			"gdisk",
			"insights-client",
			"kernel",
			"langpacks-en",
			"net-tools",
			"NetworkManager",
			"redhat-release",
			"redhat-release-eula",
			"rng-tools",
			"rsync",
			"selinux-policy-targeted",
			"tar",
			"yum-utils",

			// TODO this doesn't exist in BaseOS or AppStream
			// "rh-amazon-rhui-client",
		},
		ExcludedPackages: []string{
			"aic94xx-firmware",
			"alsa-firmware",
			"alsa-lib",
			"alsa-tools-firmware",
			"biosdevname",
			"dracut-config-rescue",
			"firewalld",
			"iprutils",
			"ivtv-firmware",
			"iwl1000-firmware",
			"iwl100-firmware",
			"iwl105-firmware",
			"iwl135-firmware",
			"iwl2000-firmware",
			"iwl2030-firmware",
			"iwl3160-firmware",
			"iwl3945-firmware",
			"iwl4965-firmware",
			"iwl5000-firmware",
			"iwl5150-firmware",
			"iwl6000-firmware",
			"iwl6000g2a-firmware",
			"iwl6000g2b-firmware",
			"iwl6050-firmware",
			"iwl7260-firmware",
			"libertas-sd8686-firmware",
			"libertas-sd8787-firmware",
			"libertas-usb8388-firmware",
			"plymouth",

			// TODO this cannot be removed, because the kernel (?)
			// depends on it. The ec2 kickstart force-removes it.
			// "linux-firmware",

			// TODO setfiles failes because of usr/sbin/timedatex. Exlude until
			// https://errata.devel.redhat.com/advisory/47339 lands
			"timedatex",
		},
		DefaultTarget: "multi-user.target",
		Bootable:      true,
		KernelOptions: "ro console=ttyS0,115200n8 console=tty0 net.ifnames=0 rd.blacklist=nouveau nvme_core.io_timeout=4294967295 crashkernel=auto",
		Assembler:     r.qemuAssembler("raw.xz", "image.raw.xz", 6*GigaByte),
	}

	r.outputs["ext4-filesystem"] = output{
		Name:     "filesystem.img",
		MimeType: "application/octet-stream",
		Packages: []string{
			"policycoreutils",
			"selinux-policy-targeted",
			"kernel",
			"firewalld",
			"chrony",
			"dracut-config-generic",
			"langpacks-en",
		},
		ExcludedPackages: []string{
			"dracut-config-rescue",

			// TODO setfiles failes because of usr/sbin/timedatex. Exlude until
			// https://errata.devel.redhat.com/advisory/47339 lands
			"timedatex",
		},
		Bootable:      false,
		KernelOptions: "ro net.ifnames=0",
		Assembler:     r.rawFSAssembler("filesystem.img"),
	}

	r.outputs["partitioned-disk"] = output{
		Name:     "disk.img",
		MimeType: "application/octet-stream",
		Packages: []string{
			"@core",
			"chrony",
			"dracut-config-generic",
			"firewalld",
			"kernel",
			"langpacks-en",
			"selinux-policy-targeted",
		},
		ExcludedPackages: []string{
			"dracut-config-rescue",

			// TODO setfiles failes because of usr/sbin/timedatex. Exlude until
			// https://errata.devel.redhat.com/advisory/47339 lands
			"timedatex",
		},
		Bootable:      true,
		KernelOptions: "ro net.ifnames=0",
		Assembler:     r.qemuAssembler("raw", "disk.img", 3*GigaByte),
	}

	r.outputs["qcow2"] = output{
		Name:     "disk.qcow2",
		MimeType: "application/x-qemu-disk",
		Packages: []string{
			"kernel-core",
			"chrony",
			"dracut-config-generic",
			"polkit",
			"systemd-udev",
			"selinux-policy-targeted",
			"langpacks-en",
		},
		ExcludedPackages: []string{
			"dracut-config-rescue",
			"etables",
			"firewalld",
			"gobject-introspection",
			"plymouth",

			// TODO setfiles failes because of usr/sbin/timedatex. Exlude until
			// https://errata.devel.redhat.com/advisory/47339 lands
			"timedatex",
		},
		Bootable:      true,
		KernelOptions: "ro net.ifnames=0",
		Assembler:     r.qemuAssembler("qcow2", "disk.qcow2", 3*GigaByte),
	}

	r.outputs["openstack"] = output{
		Name:     "image.qcow2",
		MimeType: "application/x-qemu-disk",
		Packages: []string{
			// Defaults
			"@Core",
			"langpacks-en",

			// Don't run dracut in host-only mode, in order to pull in
			// the hv_vmbus, hv_netvsc and hv_storvsc modules into the initrd.
			"dracut-config-generic",

			// From the lorax kickstart
			"kernel",
			"selinux-policy-targeted",
			"cloud-init",
			"qemu-guest-agent",
			"spice-vdagent",
		},
		ExcludedPackages: []string{
			"dracut-config-rescue",
		},
		Bootable:      true,
		KernelOptions: "ro net.ifnames=0",
		Assembler:     r.qemuAssembler("qcow2", "image.qcow2", 3*GigaByte),
	}

	r.outputs["tar"] = output{
		Name:     "root.tar.xz",
		MimeType: "application/x-tar",
		Packages: []string{
			"policycoreutils",
			"selinux-policy-targeted",
			"kernel",
			"firewalld",
			"chrony",
			"dracut-config-generic",
			"langpacks-en",
		},
		ExcludedPackages: []string{
			"dracut-config-rescue",

			// TODO setfiles failes because of usr/sbin/timedatex. Exlude until
			// https://errata.devel.redhat.com/advisory/47339 lands
			"timedatex",
		},
		Bootable:      false,
		KernelOptions: "ro net.ifnames=0",
		Assembler:     r.tarAssembler("root.tar.xz", "xz"),
	}

	r.outputs["vhd"] = output{
		Name:     "image.vhd",
		MimeType: "application/x-vhd",
		Packages: []string{
			// Defaults
			"@Core",
			"langpacks-en",

			// Don't run dracut in host-only mode, in order to pull in
			// the hv_vmbus, hv_netvsc and hv_storvsc modules into the initrd.
			"dracut-config-generic",

			// From the lorax kickstart
			"kernel",
			"selinux-policy-targeted",
			"chrony",
			"WALinuxAgent",
			"python3",
			"net-tools",
			"cloud-init",
			"cloud-utils-growpart",
			"gdisk",
		},
		ExcludedPackages: []string{
			"dracut-config-rescue",

			// TODO setfiles failes because of usr/sbin/timedatex. Exlude until
			// https://errata.devel.redhat.com/advisory/47339 lands
			"timedatex",
		},
		EnabledServices: []string{
			"sshd",
			"waagent",
		},
		DefaultTarget: "multi-user.target",
		Bootable:      true,
		KernelOptions: "ro biosdevname=0 rootdelay=300 console=ttyS0 earlyprintk=ttyS0 net.ifnames=0",
		Assembler:     r.qemuAssembler("vpc", "image.vhd", 3*GigaByte),
	}

	r.outputs["vmdk"] = output{
		Name:     "disk.vmdk",
		MimeType: "application/x-vmdk",
		Packages: []string{
			"@core",
			"chrony",
			"dracut-config-generic",
			"firewalld",
			"kernel",
			"langpacks-en",
			"open-vm-tools",
			"selinux-policy-targeted",
		},
		ExcludedPackages: []string{
			"dracut-config-rescue",

			// TODO setfiles failes because of usr/sbin/timedatex. Exlude until
			// https://errata.devel.redhat.com/advisory/47339 lands
			"timedatex",
		},
		Bootable:      true,
		KernelOptions: "ro net.ifnames=0",
		Assembler:     r.qemuAssembler("vmdk", "disk.vmdk", 3*GigaByte),
	}

	return &r
}

func (r *RHEL82) Repositories(arch string) []rpmmd.RepoConfig {
	return []rpmmd.RepoConfig{
		{
			Id:      "baseos",
			Name:    "BaseOS",
			BaseURL: "http://download-ipv4.eng.brq.redhat.com/rhel-8/nightly/RHEL-8/RHEL-8.2.0-20191213.n.1/compose/BaseOS/" + arch + "/os",
		},
		{
			Id:      "appstream",
			Name:    "AppStream",
			BaseURL: "http://download-ipv4.eng.brq.redhat.com/rhel-8/nightly/RHEL-8/RHEL-8.2.0-20191213.n.1/compose/AppStream/" + arch + "/os",
		},
	}
}

func (r *RHEL82) ListOutputFormats() []string {
	formats := make([]string, 0, len(r.outputs))
	for name := range r.outputs {
		formats = append(formats, name)
	}
	sort.Strings(formats)
	return formats
}

func (r *RHEL82) FilenameFromType(outputFormat string) (string, string, error) {
	if output, exists := r.outputs[outputFormat]; exists {
		return output.Name, output.MimeType, nil
	}
	return "", "", errors.New("invalid output format: " + outputFormat)
}

func (r *RHEL82) Pipeline(b *blueprint.Blueprint, additionalRepos []rpmmd.RepoConfig, checksums map[string]string, outputArchitecture, outputFormat string) (*pipeline.Pipeline, error) {
	output, exists := r.outputs[outputFormat]
	if !exists {
		return nil, errors.New("invalid output format: " + outputFormat)
	}

	arch, exists := r.arches[outputArchitecture]
	if !exists {
		return nil, errors.New("invalid architecture: " + outputArchitecture)
	}

	p := &pipeline.Pipeline{}
	p.SetBuild(r.buildPipeline(arch, checksums), "org.osbuild.rhel82")

	packages := append(output.Packages, b.GetPackages()...)
	if output.Bootable {
		packages = append(packages, arch.BootloaderPackages...)
	}
	p.AddStage(pipeline.NewDNFStage(r.dnfStageOptions(arch, additionalRepos, checksums, packages, output.ExcludedPackages)))
	p.AddStage(pipeline.NewFixBLSStage())

	if output.Bootable {
		p.AddStage(pipeline.NewFSTabStage(r.fsTabStageOptions()))
	}

	kernelOptions := output.KernelOptions
	if kernel := b.GetKernel(); kernel != nil {
		kernelOptions += " " + kernel.Append
	}
	p.AddStage(pipeline.NewGRUB2Stage(r.grub2StageOptions(kernelOptions)))

	// TODO support setting all languages and install corresponding langpack-* package
	language, keyboard := b.GetPrimaryLocale()

	if language != nil {
		p.AddStage(pipeline.NewLocaleStage(&pipeline.LocaleStageOptions{*language}))
	} else {
		p.AddStage(pipeline.NewLocaleStage(&pipeline.LocaleStageOptions{"en_US"}))
	}

	if keyboard != nil {
		p.AddStage(pipeline.NewKeymapStage(&pipeline.KeymapStageOptions{*keyboard}))
	}

	if hostname := b.GetHostname(); hostname != nil {
		p.AddStage(pipeline.NewHostnameStage(&pipeline.HostnameStageOptions{*hostname}))
	}

	timezone, ntpServers := b.GetTimezoneSettings()

	// TODO install chrony when this is set?
	if timezone != nil {
		p.AddStage(pipeline.NewTimezoneStage(&pipeline.TimezoneStageOptions{*timezone}))
	}

	if len(ntpServers) > 0 {
		p.AddStage(pipeline.NewChronyStage(&pipeline.ChronyStageOptions{ntpServers}))
	}

	if users := b.GetUsers(); len(users) > 0 {
		options, err := r.userStageOptions(users)
		if err != nil {
			return nil, err
		}
		p.AddStage(pipeline.NewUsersStage(options))
	}

	if groups := b.GetGroups(); len(groups) > 0 {
		p.AddStage(pipeline.NewGroupsStage(r.groupStageOptions(groups)))
	}

	if services := b.GetServices(); services != nil || output.EnabledServices != nil {
		p.AddStage(pipeline.NewSystemdStage(r.systemdStageOptions(output.EnabledServices, output.DisabledServices, services, output.DefaultTarget)))
	}

	if firewall := b.GetFirewall(); firewall != nil {
		p.AddStage(pipeline.NewFirewallStage(r.firewallStageOptions(firewall)))
	}

	p.AddStage(pipeline.NewSELinuxStage(r.selinuxStageOptions()))
	p.Assembler = output.Assembler

	return p, nil
}

func (r *RHEL82) Runner() string {
	return "org.osbuild.rhel82"
}

func (r *RHEL82) buildPipeline(arch arch, checksums map[string]string) *pipeline.Pipeline {
	packages := []string{
		"dnf",
		"dracut-config-generic",
		"e2fsprogs",
		"glibc",
		"policycoreutils",
		"python36",
		"qemu-img",
		"systemd",
		"tar",
		"xfsprogs",
	}
	packages = append(packages, arch.BuildPackages...)
	p := &pipeline.Pipeline{}
	p.AddStage(pipeline.NewDNFStage(r.dnfStageOptions(arch, nil, checksums, packages, nil)))
	return p
}

func (r *RHEL82) dnfStageOptions(arch arch, additionalRepos []rpmmd.RepoConfig, checksums map[string]string, packages, excludedPackages []string) *pipeline.DNFStageOptions {
	options := &pipeline.DNFStageOptions{
		ReleaseVersion:   "8",
		BaseArchitecture: arch.Name,
		ModulePlatformId: "platform:el8",
	}
	for _, repo := range append(r.Repositories(arch.Name), additionalRepos...) {
		options.AddRepository(&pipeline.DNFRepository{
			BaseURL:    repo.BaseURL,
			MetaLink:   repo.Metalink,
			MirrorList: repo.MirrorList,
			Checksum:   checksums[repo.Id],
		})
	}

	sort.Strings(packages)
	for _, pkg := range packages {
		options.AddPackage(pkg)
	}

	sort.Strings(excludedPackages)
	for _, pkg := range excludedPackages {
		options.ExcludePackage(pkg)
	}

	return options
}

func (r *RHEL82) userStageOptions(users []blueprint.UserCustomization) (*pipeline.UsersStageOptions, error) {
	options := pipeline.UsersStageOptions{
		Users: make(map[string]pipeline.UsersStageOptionsUser),
	}

	for _, c := range users {
		if c.Password != nil && !crypt.PasswordIsCrypted(*c.Password) {
			cryptedPassword, err := crypt.CryptSHA512(*c.Password)
			if err != nil {
				return nil, err
			}

			c.Password = &cryptedPassword
		}

		user := pipeline.UsersStageOptionsUser{
			Groups:      c.Groups,
			Description: c.Description,
			Home:        c.Home,
			Shell:       c.Shell,
			Password:    c.Password,
			Key:         c.Key,
		}

		if c.UID != nil {
			uid := strconv.Itoa(*c.UID)
			user.UID = &uid
		}

		if c.GID != nil {
			gid := strconv.Itoa(*c.GID)
			user.GID = &gid
		}

		options.Users[c.Name] = user
	}

	return &options, nil
}

func (r *RHEL82) groupStageOptions(groups []blueprint.GroupCustomization) *pipeline.GroupsStageOptions {
	options := pipeline.GroupsStageOptions{
		Groups: map[string]pipeline.GroupsStageOptionsGroup{},
	}

	for _, group := range groups {
		groupData := pipeline.GroupsStageOptionsGroup{
			Name: group.Name,
		}
		if group.GID != nil {
			gid := strconv.Itoa(*group.GID)
			groupData.GID = &gid
		}

		options.Groups[group.Name] = groupData
	}

	return &options
}

func (r *RHEL82) firewallStageOptions(firewall *blueprint.FirewallCustomization) *pipeline.FirewallStageOptions {
	options := pipeline.FirewallStageOptions{
		Ports: firewall.Ports,
	}

	if firewall.Services != nil {
		options.EnabledServices = firewall.Services.Enabled
		options.DisabledServices = firewall.Services.Disabled
	}

	return &options
}

func (r *RHEL82) systemdStageOptions(enabledServices, disabledServices []string, s *blueprint.ServicesCustomization, target string) *pipeline.SystemdStageOptions {
	if s != nil {
		enabledServices = append(enabledServices, s.Enabled...)
		enabledServices = append(disabledServices, s.Disabled...)
	}
	return &pipeline.SystemdStageOptions{
		EnabledServices:  enabledServices,
		DisabledServices: disabledServices,
		DefaultTarget:    target,
	}
}

func (r *RHEL82) fsTabStageOptions() *pipeline.FSTabStageOptions {
	options := pipeline.FSTabStageOptions{}
	options.AddFilesystem("0bd700f8-090f-4556-b797-b340297ea1bd", "xfs", "/", "defaults", 0, 0)
	return &options
}

func (r *RHEL82) grub2StageOptions(kernelOptions string) *pipeline.GRUB2StageOptions {
	id, err := uuid.Parse("0bd700f8-090f-4556-b797-b340297ea1bd")
	if err != nil {
		panic("invalid UUID")
	}
	return &pipeline.GRUB2StageOptions{
		RootFilesystemUUID: id,
		KernelOptions:      kernelOptions,
		Legacy:             true,
	}
}

func (r *RHEL82) selinuxStageOptions() *pipeline.SELinuxStageOptions {
	return &pipeline.SELinuxStageOptions{
		FileContexts: "etc/selinux/targeted/contexts/files/file_contexts",
	}
}

func (r *RHEL82) qemuAssembler(format string, filename string, size uint64) *pipeline.Assembler {
	return pipeline.NewQEMUAssembler(
		&pipeline.QEMUAssemblerOptions{
			Format:   format,
			Filename: filename,
			Size:     size,
			PTUUID:   "0x14fc63d2",
			PTType:   "mbr",
			Partitions: []pipeline.QEMUPartition{
				{
					Start:    2048,
					Bootable: true,
					Filesystem: pipeline.QEMUFilesystem{
						Type:       "xfs",
						UUID:       "0bd700f8-090f-4556-b797-b340297ea1bd",
						Mountpoint: "/",
					},
				},
			},
		},
	)
}

func (r *RHEL82) tarAssembler(filename, compression string) *pipeline.Assembler {
	return pipeline.NewTarAssembler(
		&pipeline.TarAssemblerOptions{
			Filename:    filename,
			Compression: compression,
		})
}

func (r *RHEL82) rawFSAssembler(filename string) *pipeline.Assembler {
	id, err := uuid.Parse("0bd700f8-090f-4556-b797-b340297ea1bd")
	if err != nil {
		panic("invalid UUID")
	}
	return pipeline.NewRawFSAssembler(
		&pipeline.RawFSAssemblerOptions{
			Filename:           filename,
			RootFilesystemUUDI: id,
			Size:               3221225472,
			FilesystemType:     "xfs",
		})
}
