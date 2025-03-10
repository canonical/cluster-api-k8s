package cloudinit_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestFormatAdditionalUserData(t *testing.T) {
	g := NewWithT(t)

	cases := []struct {
		name                        string
		inputAdditionalUserData     map[string]string
		formattedAdditionalUserData map[string]any
	}{
		{
			name: "AdditionalUserData",
			inputAdditionalUserData: map[string]string{
				"disk_setup": `ephemeral0:
  layout: false
  overwrite: false
  table_type: mbr`,
				"package_update": "true",
				"users": `- expiredate: "2032-09-01"
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
			formattedAdditionalUserData: map[string]any{
				"package_update": true,
				"disk_setup": map[string]any{
					"ephemeral0": map[string]any{
						"table_type": "mbr",
						"layout":     false,
						"overwrite":  false,
					},
				},
				"users": []any{
					map[string]any{
						"name":          "foobar",
						"gecos":         "Foo B. Bar",
						"primary_group": "foobar",
						"groups":        "users",
						"selinux_user":  "staff_u",
						"expiredate":    "2032-09-01",
						"ssh_import_id": []any{
							"lp:falcojr",
							"gh:TheRealFalcon",
						},
						"lock_passwd": false,
						"passwd":      "$6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/",
					},
				},
			},
		},
		{
			name: "MappingAdditionalUserData",
			inputAdditionalUserData: map[string]string{
				"disk_setup": `ephemeral0:
  table_type: mbr
  layout: False
  overwrite: False`,
			},
			formattedAdditionalUserData: map[string]any{
				"disk_setup": map[string]any{
					"ephemeral0": map[string]any{
						"layout":     false,
						"overwrite":  false,
						"table_type": "mbr",
					},
				},
			},
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
			formattedAdditionalUserData: map[string]any{
				"users": []any{
					map[string]any{
						"gecos":  "Ansible User",
						"groups": "users,admin,wheel,lxd",
						"name":   "ansible",
						"shell":  "/bin/bash",
						"sudo":   "ALL=(ALL) NOPASSWD:ALL",
					},
				},
			},
		},
		{
			name: "LiteralAdditionalUserData",
			inputAdditionalUserData: map[string]string{
				"package_update":  "true",
				"package_upgrade": "true",
			},
			formattedAdditionalUserData: map[string]any{
				"package_update":  true,
				"package_upgrade": true,
			},
		},
		{
			name: "WithManagedKeys",
			inputAdditionalUserData: map[string]string{
				"package_update":                         "true",
				cloudinit.GetManagedCloudInitFields()[0]: "some-value",
			},
			formattedAdditionalUserData: map[string]any{
				"package_update": true,
				// managed key is ignored
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			formatted := cloudinit.FormatAdditionalUserData(c.inputAdditionalUserData)
			g.Expect(c.formattedAdditionalUserData).To(Equal(formatted))
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
			name: "WithAdditionalUserData",
			config: cloudinit.CloudConfig{
				RunCommands:  []string{"runCmd"},
				BootCommands: []string{"bootCmd"},
				AdditionalUserData: map[string]any{
					"package_update": true,
					"disk_setup": map[string]any{
						"ephemeral0": map[string]any{
							"table_type": "mbr",
							"layout":     false,
							"overwrite":  false,
						},
					},
					"users": []any{
						map[string]any{
							"name":          "foobar",
							"gecos":         "Foo B. Bar",
							"primary_group": "foobar",
							"groups":        "users",
							"selinux_user":  "staff_u",
							"expiredate":    "2032-09-01",
							"ssh_import_id": []any{
								"lp:falcojr",
								"gh:TheRealFalcon",
							},
							"lock_passwd": false,
							"passwd":      "$6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/",
						},
					},
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
			cloudinitScript, err := cloudinit.GenerateCloudConfig(c.config)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(string(cloudinitScript)).To(Equal(c.expectedCloudInitScript))
		})
	}
}
