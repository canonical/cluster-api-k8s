package cloudinit_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestFormatAdditionalUserData(t *testing.T) {
	g := NewWithT(t)

	cases := []struct {
		name                        string
		inputAdditionalUserData     map[string]string
		formattedAdditionalUserData map[string]string
		expectError                 bool
	}{
		{
			name: "MappingAdditionalUserData",
			inputAdditionalUserData: map[string]string{
				"disk_setup": `ephemeral0:
    table_type: mbr
    layout: False
    overwrite: False`,
			},
			formattedAdditionalUserData: map[string]string{
				"disk_setup": `
  ephemeral0:
    layout: false
    overwrite: false
    table_type: mbr
  `,
			},
			expectError: false,
		},
		{
			name: "SequenceAdditionalUserData",
			inputAdditionalUserData: map[string]string{
				"users": `- name: ansible
  gecos: Ansible User
  shell: /bin/bash
  groups: users,admin,wheel,lxd
  sudo: ALL=(ALL) NOPASSWD:ALL`,
			},
			formattedAdditionalUserData: map[string]string{
				"users": `
- gecos: Ansible User
  groups: users,admin,wheel,lxd
  name: ansible
  shell: /bin/bash
  sudo: ALL=(ALL) NOPASSWD:ALL
`,
			},
			expectError: false,
		},
		{
			name: "LiteralAdditionalUserData",
			inputAdditionalUserData: map[string]string{
				"package_update":  "true",
				"package_upgrade": "true",
			},
			formattedAdditionalUserData: map[string]string{
				"package_update":  "true",
				"package_upgrade": "true",
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := cloudinit.FormatAdditionalUserData(context.Background(), c.inputAdditionalUserData)
			if c.expectError {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(c.inputAdditionalUserData).To(Equal(c.formattedAdditionalUserData))
		})
	}
}

func TestGenerateCloudConfig(t *testing.T) {
	g := NewWithT(t)

	cases := []struct {
		name                    string
		config                  cloudinit.CloudConfig
		expectedCloudInitScript string
	}{
		{
			name: "WithoutAdditionalUserData",
			config: cloudinit.CloudConfig{
				RunCommands:  []string{"runCmd"},
				BootCommands: []string{"bootCmd"},
			},
			expectedCloudInitScript: `## template: jinja
#cloud-config
write_files: []
runcmd:
  - runCmd
bootcmd:
  - bootCmd
`,
		},
		{
			name: "WithManagedKeysAsAdditionalUserdata",
			config: cloudinit.CloudConfig{
				RunCommands:  []string{"runCmd"},
				BootCommands: []string{"bootCmd"},
				AdditionalUserData: map[string]string{
					"runcmd": "anotherRunCmd",
				},
			},
			expectedCloudInitScript: `## template: jinja
#cloud-config
write_files: []
runcmd:
  - runCmd
bootcmd:
  - bootCmd
`,
		},
		{
			name: "WithAdditionalUserData",
			config: cloudinit.CloudConfig{
				RunCommands:  []string{"runCmd"},
				BootCommands: []string{"bootCmd"},
				AdditionalUserData: map[string]string{
					"package_update": "true",
					"disk_setup": `ephemeral0:
    table_type: mbr
    layout: False
    overwrite: False`,
					"users": `- name: foobar
  gecos: Foo B. Bar
  primary_group: foobar
  groups: users
  selinux_user: staff_u
  expiredate: '2032-09-01'
  ssh_import_id:
  - lp:falcojr
  - gh:TheRealFalcon
  lock_passwd: false
  passwd: $6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/`,
				},
			},
			expectedCloudInitScript: `## template: jinja
#cloud-config
write_files: []
runcmd:
  - runCmd
bootcmd:
  - bootCmd

disk_setup: 
  ephemeral0:
    layout: false
    overwrite: false
    table_type: mbr
  
package_update: true
users: 
- expiredate: "2032-09-01"
  gecos: Foo B. Bar
  groups: users
  lock_passwd: false
  name: foobar
  passwd: $6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/
  primary_group: foobar
  selinux_user: staff_u
  ssh_import_id:
    - lp:falcojr
    - gh:TheRealFalcon
`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cloudinitScript, err := cloudinit.GenerateCloudConfig(context.Background(), c.config)

			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(string(cloudinitScript)).To(Equal(c.expectedCloudInitScript))
		})
	}
}
