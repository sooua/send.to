// Command send is the send.to command-line client.
//
// transfer.sh users have kept a hand-rolled curl function in their shell rc
// for a decade; this replaces it with something that knows about expiry,
// download limits, encryption, resumable downloads and — the part a bare curl
// alias can never give you — a record of what you uploaded and how to delete
// it again.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/sooua/send.to/client"
)

// version is injected at build time.
var version = "dev"

func main() {
	client.Version = version

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := newApp()

	if err := app.RunContext(ctx, reorderArgs(app, os.Args)); err != nil {
		// A cancelled context means the user pressed Ctrl-C; that is not a
		// failure worth a stack of red text.
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "aborted")
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newApp() *cli.App {
	serverFlags := []cli.Flag{
		&cli.StringFlag{
			Name:    "url",
			Usage:   "server URL, overriding the configured profile",
			EnvVars: []string{"SENDTO_URL"},
		},
		&cli.StringFlag{
			Name:    "profile",
			Aliases: []string{"p"},
			Usage:   "named profile from the config file",
		},
	}

	return &cli.App{
		Name:                 "send",
		Usage:                "share files from the command line",
		Version:              version,
		EnableBashCompletion: true,
		Description: `Upload a file and get a link back:

    send put report.pdf
    send put ./build.tar.gz --days 7 --max-downloads 3

List what this machine has uploaded, and delete one again:

    send ls
    send rm https://send.to/aB3cD4eF/report.pdf

Point it at a server once and forget about it:

    send config add work https://files.example.com --default`,
		Commands: []*cli.Command{
			putCommand(serverFlags),
			getCommand(serverFlags),
			listCommand(),
			removeCommand(serverFlags),
			infoCommand(serverFlags),
			configCommand(),
		},
	}
}

// resolveClient builds a client from the flags, environment and config file.
func resolveClient(c *cli.Context) (*client.Client, client.Profile, error) {
	cfg, err := client.LoadConfig()
	if err != nil {
		return nil, client.Profile{}, err
	}

	profile, err := cfg.Resolve(c.String("url"), c.String("profile"))
	if err != nil {
		return nil, client.Profile{}, err
	}

	api := client.New(profile.URL)
	api.Username = profile.Username
	api.Password = profile.Password

	return api, profile, nil
}

// ---------------------------------------------------------------- put

func putCommand(serverFlags []cli.Flag) *cli.Command {
	return &cli.Command{
		Name:      "put",
		Aliases:   []string{"up"},
		Usage:     "upload one or more files (use - for stdin)",
		ArgsUsage: "<file>...",
		Flags: append([]cli.Flag{
			&cli.IntFlag{
				Name:    "days",
				Aliases: []string{"d"},
				Usage:   "delete after N days",
			},
			&cli.IntFlag{
				Name:    "max-downloads",
				Aliases: []string{"n"},
				Usage:   "delete after N completed downloads",
			},
			&cli.BoolFlag{
				Name:  "e2e",
				Usage: "encrypt on this machine; the key rides in the link fragment and never reaches the server",
			},
			&cli.StringFlag{
				Name:    "encrypt",
				Aliases: []string{"e"},
				Usage:   "server-side encryption with this password (the server sees the plaintext)",
			},
			&cli.StringFlag{
				Name:  "name",
				Usage: "override the stored filename (required with -)",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "print the full server response instead of the URL",
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "print only the URL, no progress",
			},
		}, serverFlags...),
		Action: runPut,
	}
}

func runPut(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("nothing to upload — pass one or more files, or - for stdin")
	}

	if err := rejectFlagLikeArgs(c.Args().Slice()); err != nil {
		return err
	}

	api, profile, err := resolveClient(c)
	if err != nil {
		return err
	}

	history, err := client.LoadHistory()
	if err != nil {
		// History is a convenience; never block an upload on it.
		fmt.Fprintln(os.Stderr, "warning: could not read history:", err)
		history = &client.History{}
	}

	opts := client.UploadOptions{
		Days:         c.Int("days"),
		MaxDownloads: c.Int("max-downloads"),
		Password:     c.String("encrypt"),
	}

	quiet := c.Bool("quiet")

	for _, arg := range c.Args().Slice() {
		result, err := uploadOne(c.Context, api, profile.URL, arg, c.String("name"), opts, quiet, c.Bool("e2e"))
		if err != nil {
			return err
		}

		history.Add(profile.URL, result)

		if c.Bool("json") {
			if err := printJSON(result); err != nil {
				return err
			}
			continue
		}

		fmt.Println(result.URL)

		if !quiet {
			printUploadSummary(result)
		}
	}

	if err := history.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not save history:", err)
	}

	return nil
}

func uploadOne(ctx context.Context, api *client.Client, server, arg, nameOverride string, opts client.UploadOptions, quiet, e2e bool) (*client.Result, error) {
	name := nameOverride

	if arg == "-" {
		if name == "" {
			return nil, errors.New("uploading from stdin needs --name")
		}
		// Length is unknown, so the server buffers it to disk to learn one.
		return uploadStream(ctx, api, name, os.Stdin, -1, opts, e2e)
	}

	f, err := os.Open(arg) //nolint:gosec // the path is the user's own argument
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if name == "" {
		name = filepath.Base(arg)
	}

	size := client.FileSize(f)

	// Big enough to be worth the extra round trips: an interrupted transfer
	// resumes instead of starting over. A server without the endpoint falls
	// back to a plain PUT, so this stays safe against older instances.
	if size >= client.ResumableThreshold {
		result, err := uploadResumable(ctx, api, server, arg, f, name, size, opts, quiet, e2e)
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, client.ErrResumableUnsupported) {
			return nil, err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}

	var body io.Reader = f
	if !quiet && size > 0 {
		bar := newProgressBar(name, size)
		defer bar.done()
		body = bar.wrap(f)
	}

	return uploadStream(ctx, api, name, body, size, opts, e2e)
}

// uploadStream sends the body, encrypting it here first when --e2e is set. The
// returned URL then carries the key in its fragment; the server only ever sees
// ciphertext and has no way to read it.
func uploadStream(ctx context.Context, api *client.Client, name string, body io.Reader, size int64, opts client.UploadOptions, e2e bool) (*client.Result, error) {
	if !e2e {
		return api.Upload(ctx, name, body, size, opts)
	}

	if opts.Password != "" {
		return nil, errors.New("--e2e and --encrypt are alternatives: --e2e keeps the key on this machine, --encrypt hands it to the server")
	}

	key, err := client.NewE2EKey()
	if err != nil {
		return nil, err
	}

	meta := client.E2EMetadata{Name: name, Type: contentTypeFor(name)}

	encrypted, err := client.E2EEncrypt(body, key, meta)
	if err != nil {
		return nil, err
	}

	// The ciphertext length is exactly derivable, so the upload still declares
	// a Content-Length and the server need not spool it to disk to find one.
	encryptedSize := int64(-1)
	if size >= 0 {
		if encryptedSize, err = client.E2EEncryptedSize(size, meta); err != nil {
			return nil, err
		}
	}

	result, err := api.Upload(ctx, name, encrypted, encryptedSize, opts)
	if err != nil {
		return nil, err
	}

	result.URL += "#k=" + key.String()
	result.Encrypted = true
	result.Size = size

	return result, nil
}

func contentTypeFor(name string) string {
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func printUploadSummary(r *client.Result) {
	var parts []string

	if r.ExpiresAt != nil {
		parts = append(parts, "expires "+r.ExpiresAt.Local().Format("2006-01-02 15:04"))
	}
	if r.MaxDownloads != nil {
		parts = append(parts, fmt.Sprintf("%d download(s)", *r.MaxDownloads))
	}
	if r.Encrypted {
		parts = append(parts, "encrypted")
	}

	if len(parts) > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", strings.Join(parts, " · "))
	}
	if r.DeleteURL != "" {
		fmt.Fprintf(os.Stderr, "  delete: send rm %s\n", client.StripFragment(r.URL))
	}
	if strings.Contains(r.URL, "#k=") {
		fmt.Fprintln(os.Stderr, "  everything after # is the key — without it nobody, the server included, can read this file")
	}
}

// ---------------------------------------------------------------- get

func getCommand(serverFlags []cli.Flag) *cli.Command {
	return &cli.Command{
		Name:      "get",
		Aliases:   []string{"down"},
		Usage:     "download a file, resuming if it was interrupted",
		ArgsUsage: "<url>",
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "write to this path (default: the file's own name, - for stdout)",
			},
			&cli.StringFlag{
				Name:    "decrypt",
				Aliases: []string{"e"},
				Usage:   "password for a server-encrypted upload",
			},
			&cli.BoolFlag{
				Name:  "no-resume",
				Usage: "start over instead of continuing a partial download",
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "no progress output",
			},
		}, serverFlags...),
		Action: runGet,
	}
}

func runGet(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("usage: send get <url>")
	}

	fileURL := c.Args().First()

	// The key lives in the fragment. Take it, then strip it: a fragment is
	// never meant to reach a server, and building the request from the bare
	// URL makes that structural rather than a matter of trust.
	key, err := client.FragmentKey(fileURL)
	if err != nil {
		return err
	}
	fileURL = client.StripFragment(fileURL)

	api, _, resolveErr := resolveClient(c)
	if resolveErr != nil {
		// A full URL carries its own host, so a missing profile is fine.
		if !strings.HasPrefix(fileURL, "http://") && !strings.HasPrefix(fileURL, "https://") {
			return resolveErr
		}
		api = client.New("")
	}

	opts := client.DownloadOptions{Password: c.String("decrypt")}

	// Straight to stdout: no resume, no progress, no temp file.
	if c.String("output") == "-" {
		if key == nil {
			_, _, err := api.Download(c.Context, fileURL, os.Stdout, opts)
			return err
		}
		return downloadDecryptedToStdout(c, api, fileURL, key, opts)
	}

	dest := c.String("output")
	if dest == "" {
		dest = filepath.Base(strings.TrimSuffix(fileURL, "/"))
		if unescaped, uErr := unescapePath(dest); uErr == nil {
			dest = unescaped
		}
	}

	// Downloads land in <dest>.part and are renamed on success, so an
	// interrupted run leaves something to resume from and never a truncated
	// file masquerading as a complete one.
	partial := dest + ".part"

	flags := os.O_CREATE | os.O_WRONLY
	if !c.Bool("no-resume") {
		if info, statErr := os.Stat(partial); statErr == nil {
			opts.Offset = info.Size()
			flags |= os.O_APPEND
		}
	} else {
		flags |= os.O_TRUNC
	}

	f, err := os.OpenFile(partial, flags, 0600) //nolint:gosec // the path is derived from the user's argument
	if err != nil {
		return err
	}

	if opts.Offset > 0 && !c.Bool("quiet") {
		fmt.Fprintf(os.Stderr, "resuming at %s\n", humanBytes(opts.Offset))
	}

	var bar *progressBar
	if !c.Bool("quiet") {
		opts.Progress = func(written, total int64) {
			if bar == nil {
				bar = newProgressBar(filepath.Base(dest), total)
			}
			bar.set(written)
		}
	}

	_, resumed, err := api.Download(c.Context, fileURL, f, opts)
	if bar != nil {
		bar.done()
	}

	if err != nil {
		_ = f.Close()
		return err
	}

	// The server ignored our offset and restarted, so what we appended is
	// duplicated on top of the old bytes. Redo it from scratch rather than
	// hand back a corrupt file.
	if opts.Offset > 0 && !resumed {
		_ = f.Close()
		_ = os.Remove(partial)
		return errors.New("server restarted the transfer instead of resuming; run again with --no-resume")
	}

	if err := f.Close(); err != nil {
		return err
	}

	// Encrypted downloads resume as ciphertext and are decrypted once whole,
	// so an interrupted transfer still picks up where it left off.
	if key != nil {
		name, err := decryptFile(partial, dest, c.String("output"), key)
		if err != nil {
			return err
		}
		dest = name
	} else if err := os.Rename(partial, dest); err != nil {
		return err
	}

	if !c.Bool("quiet") {
		fmt.Fprintf(os.Stderr, "saved to %s\n", dest)
	}

	return nil
}

// decryptFile turns the downloaded ciphertext into the plaintext file, using
// the name recorded inside the encrypted metadata unless one was given.
func decryptFile(partial, dest, explicitOutput string, key client.E2EKey) (string, error) {
	in, err := os.Open(partial) //nolint:gosec // path built from the user's own argument
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()

	meta, plaintext, err := client.E2EDecrypt(in, key)
	if err != nil {
		return "", err
	}

	// The real filename is inside the ciphertext, so it need not match the URL.
	if explicitOutput == "" && meta.Name != "" {
		dest = filepath.Base(meta.Name)
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600) //nolint:gosec // as above
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(out, plaintext); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return "", err
	}

	if err := out.Close(); err != nil {
		return "", err
	}

	_ = in.Close()
	_ = os.Remove(partial)

	return dest, nil
}

// downloadDecryptedToStdout streams the ciphertext through the decrypter
// without ever putting the whole file on disk or in memory.
func downloadDecryptedToStdout(c *cli.Context, api *client.Client, fileURL string, key client.E2EKey, opts client.DownloadOptions) error {
	pr, pw := io.Pipe()

	go func() {
		_, _, err := api.Download(c.Context, fileURL, pw, opts)
		_ = pw.CloseWithError(err)
	}()

	_, plaintext, err := client.E2EDecrypt(pr, key)
	if err != nil {
		return err
	}

	_, err = io.Copy(os.Stdout, plaintext)
	return err
}

// ---------------------------------------------------------------- ls

func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "ls",
		Usage: "list uploads made from this machine",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "include entries whose expiry has passed",
			},
			&cli.BoolFlag{
				Name:  "urls",
				Usage: "print only URLs, one per line",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "print the raw history",
			},
			&cli.BoolFlag{
				Name:  "prune",
				Usage: "drop expired entries from the history",
			},
		},
		Action: runList,
	}
}

func runList(c *cli.Context) error {
	history, err := client.LoadHistory()
	if err != nil {
		return err
	}

	if c.Bool("prune") {
		removed := history.Prune()
		if err := history.Save(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "removed %d expired entr%s\n", removed, plural(removed, "y", "ies"))
		return nil
	}

	entries := history.Entries
	if !c.Bool("all") {
		kept := make([]client.Entry, 0, len(entries))
		for _, e := range entries {
			if !e.Expired() {
				kept = append(kept, e)
			}
		}
		entries = kept
	}

	if c.Bool("json") {
		return printJSON(entries)
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "nothing uploaded from this machine yet")
		return nil
	}

	if c.Bool("urls") {
		for _, e := range entries {
			fmt.Println(e.URL)
		}
		return nil
	}

	for _, e := range entries {
		expiry := "never"
		if e.ExpiresAt != nil {
			expiry = e.ExpiresAt.Local().Format("2006-01-02")
			if e.Expired() {
				expiry += " (expired)"
			}
		}

		limit := "unlimited"
		if e.MaxDownloads != nil {
			limit = fmt.Sprintf("%d max", *e.MaxDownloads)
		}

		flags := ""
		if e.Encrypted {
			flags = " [encrypted]"
		}

		fmt.Printf("%-28s  %9s  %-12s  %-10s%s\n  %s\n",
			truncate(e.Filename, 28), humanBytes(e.Size), expiry, limit, flags, e.URL)
	}

	return nil
}

// ---------------------------------------------------------------- rm

func removeCommand(serverFlags []cli.Flag) *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Aliases:   []string{"delete"},
		Usage:     "delete an upload using the stored deletion link",
		ArgsUsage: "<url>...",
		Flags: append([]cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "delete every upload in the history",
			},
		}, serverFlags...),
		Action: runRemove,
	}
}

func runRemove(c *cli.Context) error {
	history, err := client.LoadHistory()
	if err != nil {
		return err
	}

	targets := c.Args().Slice()
	if c.Bool("all") {
		targets = nil
		for _, e := range history.Entries {
			targets = append(targets, e.URL)
		}
	}

	if len(targets) == 0 {
		return errors.New("usage: send rm <url>... (or --all)")
	}

	api, _, err := resolveClient(c)
	if err != nil {
		api = client.New("")
	}

	var failed int

	for _, target := range targets {
		entry := history.Find(target)
		if entry == nil {
			// History records the full link including its key fragment.
			entry = history.Find(client.StripFragment(target))
		}
		deleteURL := client.StripFragment(target)

		switch {
		case entry != nil && entry.DeleteURL != "":
			deleteURL = entry.DeleteURL
		case strings.Count(strings.TrimPrefix(target, "https://"), "/") >= 3:
			// Already a deletion URL: /{token}/{filename}/{deletionToken}.
		default:
			fmt.Fprintf(os.Stderr, "%s: no deletion link on record — pass the delete URL itself\n", target)
			failed++
			continue
		}

		if err := api.Delete(c.Context, deleteURL); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", target, err)
			failed++
			continue
		}

		history.Remove(target)
		fmt.Fprintf(os.Stderr, "deleted %s\n", target)
	}

	if err := history.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not save history:", err)
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d deletions failed", failed, len(targets))
	}

	return nil
}

// ---------------------------------------------------------------- info

func infoCommand(serverFlags []cli.Flag) *cli.Command {
	return &cli.Command{
		Name:      "info",
		Aliases:   []string{"stat"},
		Usage:     "show a file's size and remaining limits without downloading it",
		ArgsUsage: "<url>",
		Flags:     serverFlags,
		Action:    runInfo,
	}
}

func runInfo(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("usage: send info <url>")
	}

	api, _, err := resolveClient(c)
	if err != nil {
		api = client.New("")
	}

	info, err := api.Stat(c.Context, client.StripFragment(c.Args().First()))
	if err != nil {
		if client.NotFound(err) {
			return errors.New("not available — expired, out of downloads, or deleted")
		}
		return err
	}

	fmt.Printf("filename            %s\n", info.Filename)
	fmt.Printf("size                %s\n", humanBytes(info.Size))
	fmt.Printf("content type        %s\n", info.ContentType)
	fmt.Printf("downloads remaining %s\n", info.RemainingDownloads)
	fmt.Printf("days remaining      %s\n", info.RemainingDays)
	fmt.Printf("resumable           %t\n", info.SupportsRange)

	return nil
}

// ---------------------------------------------------------------- config

func configCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "manage server profiles",
		Subcommands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "add or replace a profile",
				ArgsUsage: "<name> <url>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "user", Usage: "HTTP basic auth username"},
					&cli.StringFlag{Name: "pass", Usage: "HTTP basic auth password"},
					&cli.BoolFlag{Name: "default", Usage: "make this the default profile"},
				},
				Action: runConfigAdd,
			},
			{
				Name:      "rm",
				Usage:     "remove a profile",
				ArgsUsage: "<name>",
				Action:    runConfigRemove,
			},
			{
				Name:   "ls",
				Usage:  "list profiles",
				Action: runConfigList,
			},
			{
				Name:      "use",
				Usage:     "set the default profile",
				ArgsUsage: "<name>",
				Action:    runConfigUse,
			},
			{
				Name:   "path",
				Usage:  "print the config directory",
				Action: runConfigPath,
			},
		},
	}
}

func runConfigAdd(c *cli.Context) error {
	if c.NArg() != 2 {
		return errors.New("usage: send config add <name> <url>")
	}

	cfg, err := client.LoadConfig()
	if err != nil {
		return err
	}

	name := c.Args().Get(0)
	cfg.Profiles[name] = client.Profile{
		URL:      strings.TrimSuffix(c.Args().Get(1), "/"),
		Username: c.String("user"),
		Password: c.String("pass"),
	}

	// The first profile added is the default; there is nothing else it could
	// sensibly be.
	if c.Bool("default") || cfg.Default == "" {
		cfg.Default = name
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "saved profile %q", name)
	if cfg.Default == name {
		fmt.Fprint(os.Stderr, " (default)")
	}
	fmt.Fprintln(os.Stderr)

	if c.String("pass") != "" {
		dir, _ := client.ConfigDir()
		fmt.Fprintf(os.Stderr, "note: the password is stored in plain text in %s\n", filepath.Join(dir, "config.json"))
	}

	return nil
}

func runConfigRemove(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("usage: send config rm <name>")
	}

	cfg, err := client.LoadConfig()
	if err != nil {
		return err
	}

	name := c.Args().First()
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("no profile named %q", name)
	}

	delete(cfg.Profiles, name)
	if cfg.Default == name {
		cfg.Default = ""
		if names := cfg.ProfileNames(); len(names) > 0 {
			cfg.Default = names[0]
		}
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "removed profile %q\n", name)
	return nil
}

func runConfigList(c *cli.Context) error {
	cfg, err := client.LoadConfig()
	if err != nil {
		return err
	}

	names := cfg.ProfileNames()
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "no profiles configured — run `send config add <name> <url>`")
		return nil
	}

	for _, name := range names {
		p := cfg.Profiles[name]
		marker := " "
		if name == cfg.Default {
			marker = "*"
		}
		auth := ""
		if p.Username != "" {
			auth = "  (auth: " + p.Username + ")"
		}
		fmt.Printf("%s %-16s %s%s\n", marker, name, p.URL, auth)
	}

	return nil
}

func runConfigUse(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("usage: send config use <name>")
	}

	cfg, err := client.LoadConfig()
	if err != nil {
		return err
	}

	name := c.Args().First()
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("no profile named %q", name)
	}

	cfg.Default = name
	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "default profile is now %q\n", name)
	return nil
}

func runConfigPath(_ *cli.Context) error {
	dir, err := client.ConfigDir()
	if err != nil {
		return err
	}
	fmt.Println(dir)
	return nil
}

// ---------------------------------------------------------------- helpers

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// rejectFlagLikeArgs guards against an option that reordering did not
// recognise being taken for a filename.
func rejectFlagLikeArgs(args []string) error {
	for _, arg := range args {
		if arg != "-" && strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown option %q — run `send put --help`", arg)
		}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < 0 {
		return "?"
	}
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTP"[exp])
}

func unescapePath(s string) (string, error) {
	return url.PathUnescape(s)
}
