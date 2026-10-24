package config

import "errors"

// ValidateRecoveryMetadata validates a generated skeleton before writing secrets.
// Full Validate must still run after private credential files exist.
func ValidateRecoveryMetadata(cfg Config) error {
	if cfg.Version != CurrentVersion || !cfg.RecoveryOnly || len(cfg.Projects) != 1 || !safeName.MatchString(cfg.Projects[0].Name) || len(cfg.Projects[0].Paths) != 1 {
		return errors.New("invalid recovery-only project metadata")
	}
	if err := validateRepository(cfg.Repository); err != nil {
		return err
	}
	for _, p := range []string{cfg.StateDir, cfg.PasswordFile, cfg.Projects[0].Paths[0]} {
		if err := ValidateDestination(p); err != nil {
			return err
		}
	}
	if cfg.RcloneConfigFile != "" {
		return ValidateDestination(cfg.RcloneConfigFile)
	}
	return nil
}
