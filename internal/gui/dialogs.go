package gui

import (
	"fmt"
	"strings"

	"github.com/PeronGH/singer/internal/store"
	"github.com/egoist/mygo/ui"
)

func (a *App) dialogsView(c *ui.Context) {
	a.addRemoteDialogView(c)
	a.localNodeDialogView(c)
	a.deleteRemoteAlertView(c)
}

func (a *App) addRemoteDialogView(c *ui.Context) {
	ui.Modal(c, &a.remoteModalOpen, func() {
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Text(c, "Add Remote Subscription").FontSize(16).Bold()

			submit := false
			ui.Field(c, "Name", func() {
				in := ui.TextInput(c.Key("remote-name"), &a.remoteName).
					Placeholder("e.g. work").
					AutoFocus()
				if in.Changed() {
					a.remoteNameErr = ""
				}
				if in.Submitted() {
					submit = true
				}
			}).Error(a.remoteNameErr)

			ui.Field(c, "URL", func() {
				in := ui.TextInput(c.Key("remote-url"), &a.remoteURL).
					Placeholder("https://example.com/subscription")
				if in.Changed() {
					a.remoteURLErr = ""
				}
				if in.Submitted() {
					submit = true
				}
			}).Error(a.remoteURLErr)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.remoteModalOpen = false
				}
				if ui.PrimaryButton(c, "Add Remote").Clicked() {
					submit = true
				}
			})

			if submit {
				name := strings.TrimSpace(a.remoteName)
				remoteURL := strings.TrimSpace(a.remoteURL)
				a.remoteNameErr = ""
				a.remoteURLErr = ""

				if err := store.ValidateRemoteName(name); err != nil {
					a.remoteNameErr = err.Error()
				}
				if remoteURL == "" {
					a.remoteURLErr = "remote URL cannot be empty"
				}
				if a.remoteNameErr == "" && a.remoteURLErr == "" {
					a.remoteModalOpen = false
					a.startFetchRemote(name, remoteURL)
				}
			}
		})
	})
}

func (a *App) localNodeDialogView(c *ui.Context) {
	ui.Modal(c, &a.nodeModalOpen, func() {
		title := "Add Local Node"
		actionLabel := "Add"
		if a.editingNodeIndex >= 0 {
			title = "Edit Local Node"
			actionLabel = "Save"
		}

		ui.Column(c).Width(460).Gap(12).Children(func() {
			ui.Text(c, title).FontSize(16).Bold()

			submit := false
			ui.Field(c, "Node URI", func() {
				in := ui.TextInput(c.Key("local-node-uri"), &a.nodeURI).
					Placeholder("trojan://..., hy2://..., socks5://...")
				in.AutoFocus()
				if in.Changed() {
					a.nodeURIErr = ""
				}
				if in.Submitted() {
					submit = true
				}
			}).Error(a.nodeURIErr)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.nodeModalOpen = false
					a.editingNodeIndex = -1
				}
				if ui.PrimaryButton(c, actionLabel).Clicked() {
					submit = true
				}
			})

			if submit {
				uri := strings.TrimSpace(a.nodeURI)
				if uri == "" {
					a.nodeURIErr = "node URI cannot be empty"
					return
				}
				local := a.localGroup()
				if local == nil {
					a.nodeURIErr = "local group not loaded"
					return
				}

				if a.editingNodeIndex >= 0 {
					if a.editingNodeIndex < len(local.Nodes) {
						nodes := append([]string(nil), local.Nodes...)
						nodes[a.editingNodeIndex] = uri
						if a.saveLocalNodes(nodes, "Updated local node") {
							a.nodeModalOpen = false
							a.editingNodeIndex = -1
						}
					}
					return
				}

				nodes := append(append([]string(nil), local.Nodes...), uri)
				if a.saveLocalNodes(nodes, "Added local node") {
					a.nodeModalOpen = false
				}
			}
		})
	})
}

func (a *App) deleteRemoteAlertView(c *ui.Context) {
	name := a.confirmDeleteRemoteName
	choice := ui.AlertDialog(
		c,
		&a.confirmDeleteRemoteOpen,
		fmt.Sprintf("Delete remote %q?", name),
		"This removes the saved subscription URL and its cached nodes.",
		"Cancel",
		"Delete",
	)
	if choice == 1 {
		a.confirmDeleteRemoteName = ""
		a.deleteRemote(name)
	}
}
