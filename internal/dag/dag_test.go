package dag

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yohamta/dagu/internal/settings"
	"github.com/yohamta/dagu/internal/utils"
)

var (
	testdataDir = path.Join(utils.MustGetwd(), "testdata")
	testHomeDir = path.Join(utils.MustGetwd(), "testdata/home")
	testEnv     = []string{}
)

func TestMain(m *testing.M) {
	settings.ChangeHomeDir(testHomeDir)
	testEnv = []string{
		fmt.Sprintf("LOG_DIR=%s", path.Join(testHomeDir, "/logs")),
		fmt.Sprintf("PATH=%s", os.ExpandEnv("${PATH}")),
	}
	code := m.Run()
	os.Exit(code)
}

func TestAssertDefinition(t *testing.T) {
	l := &Loader{}

	_, err := l.Load(path.Join(testdataDir, "err_no_steps.yaml"), "")
	require.Equal(t, err, fmt.Errorf("at least one step must be specified"))
}

func TestAssertStepDefinition(t *testing.T) {
	l := &Loader{}

	_, err := l.Load(path.Join(testdataDir, "err_step_no_name.yaml"), "")
	require.Equal(t, err, fmt.Errorf("step name must be specified"))

	_, err = l.Load(path.Join(testdataDir, "err_step_no_command.yaml"), "")
	require.Equal(t, err, fmt.Errorf("step command must be specified"))
}

func TestConfigReadClone(t *testing.T) {
	l := &Loader{}

	d, err := l.Load(path.Join(testdataDir, "default.yaml"), "")
	require.NoError(t, err)

	dd := d.Clone()
	require.Equal(t, d, dd)
}

func TestToString(t *testing.T) {
	l := &Loader{}

	d, err := l.Load(path.Join(testdataDir, "default.yaml"), "")
	require.NoError(t, err)

	ret := d.String()
	require.Contains(t, ret, "Name: default")
}

func TestReadConfig(t *testing.T) {
	tmpDir := utils.MustTempDir("read-config-test")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tmpFile := path.Join(tmpDir, "config.yaml")
	testConfig := `steps:
  - name: step 1
    command: echo test
`
	err := os.WriteFile(tmpFile, []byte(testConfig), 0644)
	require.NoError(t, err)

	ret, err := ReadConfig(tmpFile)
	require.NoError(t, err)
	require.Equal(t, testConfig, ret)
}

func TestConfigLoadHeadOnly(t *testing.T) {
	l := &Loader{}

	d, err := l.LoadHeadOnly(path.Join(testdataDir, "default.yaml"))
	require.NoError(t, err)

	require.Equal(t, d.Name, "default")
	require.True(t, len(d.Steps) == 0)
}

func TestLoadInvalidConfigError(t *testing.T) {
	for _, c := range []string{
		`env: 
  VAR: "` + "`ech 1`" + `"
`,
		`logDir: "` + "`ech foo`" + `"`,
		`params: "` + "`ech foo`" + `"`,
		`schedule: "` + "1" + `"`,
	} {
		l := &Loader{}
		d, err := l.unmarshalData([]byte(c))
		require.NoError(t, err)

		def, err := l.decode(d)
		require.NoError(t, err)

		b := &builder{}
		_, err = b.buildFromDefinition(def, nil)
		require.Error(t, err)
	}
}

func TestLoadEnv(t *testing.T) {
	for _, c := range []struct {
		val, key, want string
	}{
		{
			`env: 
  VAR: "` + "`echo 1`" + `"
`,
			"VAR", "1",
		},
		{
			`env: 
  "1": "123"
`,
			"1", "123",
		},
		{
			`env: 
  - "FOO": "BAR"
  - "FOO": "${FOO}:BAZ"
  - "FOO": "${FOO}:BAR"
  - "FOO": "${FOO}:FOO"
`,
			"FOO", "BAR:BAZ:BAR:FOO",
		},
	} {
		l := &Loader{}
		d, err := l.unmarshalData([]byte(c.val))
		require.NoError(t, err)

		def, err := l.decode(d)
		require.NoError(t, err)

		b := &builder{}
		_, err = b.buildFromDefinition(def, nil)
		require.NoError(t, err)

		require.Equal(t, c.want, os.Getenv(c.key))
	}
}

func TestParseParameter(t *testing.T) {
	for _, test := range []struct {
		Params string
		Env    string
		Want   map[string]string
	}{
		{
			Params: "x",
			Want: map[string]string{
				"1": "x",
			},
		},
		{
			Params: "x y",
			Want: map[string]string{
				"1": "x",
				"2": "y",
			},
		},
		{
			Params: "x yy zzz",
			Want: map[string]string{
				"1": "x",
				"2": "yy",
				"3": "zzz",
			},
		},
		{
			Params: "x $1",
			Want: map[string]string{
				"1": "x",
				"2": "x",
			},
		},
		{
			Params: "first P1=foo P2=${FOO} P3=`/bin/echo ${P2}` X=bar Y=${P1} Z=\"A B C\"",
			Env:    "FOO: BAR",
			Want: map[string]string{
				"P1": "foo",
				"P2": "BAR",
				"P3": "BAR",
				"X":  "bar",
				"Y":  "foo",
				"Z":  "A B C",
				"1":  "first",
				"2":  "P1=foo",
				"3":  "P2=BAR",
				"4":  "P3=BAR",
				"5":  "X=bar",
				"6":  "Y=foo",
				"7":  "Z=A B C",
			},
		},
	} {
		l := &Loader{}
		d, err := l.unmarshalData([]byte(fmt.Sprintf(`
env:
  - %s
params: %s
  	`, test.Env, test.Params)))
		require.NoError(t, err)

		def, err := l.decode(d)
		require.NoError(t, err)

		b := &builder{}
		_, err = b.buildFromDefinition(def, nil)
		require.NoError(t, err)

		for k, v := range test.Want {
			require.Equal(t, v, os.Getenv(k))
		}
	}
}

func TestExpandEnv(t *testing.T) {
	b := &builder{}
	os.Setenv("FOO", "BAR")
	require.Equal(t, b.expandEnv("${FOO}"), "BAR")

	b.noEval = true
	require.Equal(t, b.expandEnv("${FOO}"), "${FOO}")
}

func TestTags(t *testing.T) {
	tags := "Daily, Monthly"
	wants := []string{"daily", "monthly"}
	l := &Loader{}
	m, err := l.unmarshalData([]byte(fmt.Sprintf(`
tags: %s
  	`, tags)))
	require.NoError(t, err)

	def, err := l.decode(m)
	require.NoError(t, err)

	b := &builder{}
	d, err := b.buildFromDefinition(def, nil)
	require.NoError(t, err)

	require.Equal(t, wants, d.Tags)

	require.True(t, d.HasTag("daily"))
	require.False(t, d.HasTag("weekly"))
}

func TestSchedule(t *testing.T) {
	for _, tc := range []struct {
		Name string
		Def  string
		Err  bool
		Want int
	}{
		{
			Name: "basic schedule",
			Def:  "schedule: \"*/5 * * * *\"",
			Want: 1,
		},
		{
			Name: "multiple schedule",
			Def: `schedule:
  - "*/5 * * * *"
  - "* * * * *"`,
			Want: 2,
		},
		{
			Name: "parsing error",
			Def: `schedule:
  - true 
  - "* * * * *"`,
			Err: true,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			l := &Loader{}
			m, err := l.unmarshalData([]byte(tc.Def))
			require.NoError(t, err)

			def, err := l.decode(m)
			require.NoError(t, err)

			b := &builder{}
			d, err := b.buildFromDefinition(def, nil)

			if tc.Err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.Want, len(d.Schedule))
			}
		})
	}
}

func TestScheduleStop(t *testing.T) {
	for _, tc := range []struct {
		Name        string
		Def         string
		Err         bool
		WantStart   int
		WantStop    int
		WantRestart int
	}{
		{
			Name: "start and stop are parsed",
			Def: `schedule:
  start: "0 1 * * *"
  stop: "0 2 * * *"
`,
			WantStart: 1,
			WantStop:  1,
		},
		{
			Name: "start only",
			Def: `schedule:
  start: "0 1 * * *"
`,
			WantStart: 1,
			WantStop:  0,
		},
		{
			Name: "stop only",
			Def: `schedule:
  stop: "0 1 * * *"
`,
			WantStart: 0,
			WantStop:  1,
		},
		{
			Name: "multiple schedule",
			Def: `schedule:
  start: 
    - "0 1 * * *"
    - "0 18 * * *"
  stop:
    - "0 2 * * *"
    - "0 20 * * *"
`,
			WantStart: 2,
			WantStop:  2,
		},
		{
			Name: "restart",
			Def: `schedule:
  start: "0 8 * * *"
  restart: "0 12 * * *"
  stop: "0 20 * * *"
`,
			WantStart:   1,
			WantStop:    1,
			WantRestart: 1,
		},
		{
			Name: "invalid expression",
			Def: `schedule:
  stop: "* * * * * * *"
`,
			Err: true,
		},
		{
			Name: "invalid key",
			Def: `schedule:
  invalid: "* * * * * * *"
`,
			Err: true,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			l := &Loader{}
			m, err := l.unmarshalData([]byte(tc.Def))
			require.NoError(t, err)

			def, err := l.decode(m)
			require.NoError(t, err)

			b := &builder{}
			d, err := b.buildFromDefinition(def, nil)

			if tc.Err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.WantStart, len(d.Schedule))
			require.Equal(t, tc.WantStop, len(d.StopSchedule))
			require.Equal(t, tc.WantRestart, len(d.RestartSchedule))
		})
	}
}

func TestSockAddr(t *testing.T) {
	d := &DAG{Location: "testdata/testDag.yml"}
	require.Regexp(t, `^/tmp/@dagu-testDag-[0-9a-f]+\.sock$`, d.SockAddr())
}

func TestOverwriteGlobalConfig(t *testing.T) {
	l := &Loader{BaseConfig: settings.MustGet(settings.SETTING__BASE_CONFIG)}

	d, err := l.Load(path.Join(testdataDir, "overwrite.yaml"), "")
	require.NoError(t, err)

	require.Equal(t, &MailOn{Failure: false, Success: false}, d.MailOn)
	require.Equal(t, d.HistRetentionDays, 7)

	d, err = l.Load(path.Join(testdataDir, "no_overwrite.yaml"), "")
	require.NoError(t, err)

	require.Equal(t, &MailOn{Failure: true, Success: false}, d.MailOn)
	require.Equal(t, d.HistRetentionDays, 30)
}
