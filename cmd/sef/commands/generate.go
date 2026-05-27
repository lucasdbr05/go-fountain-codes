package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/lucasdbr05/sef-golang/chain"
	"github.com/lucasdbr05/sef-golang/distribution"
	"github.com/lucasdbr05/sef-golang/droplet"
	"github.com/lucasdbr05/sef-golang/encode"
	"github.com/lucasdbr05/sef-golang/superblock"
	"github.com/spf13/cobra"
)

func GenerateCmd() *cobra.Command {
	var blocksDir string
	var outFile string
	var c float64
	var delta float64
	var numDroplets uint64
	var epochSize int
	var network string
	var superblockSize int

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Encode Bitcoin blocks into LT-code droplets and write to a file",
		RunE: func(cmd *cobra.Command, args []string) error {
			magic, err := parseNetwork(network)
			if err != nil {
				return err
			}
			reader, err := chain.NewBlkDatReaderAutoXor(blocksDir, magic)
			if err != nil {
				return fmt.Errorf("initializing reader: %w", err)
			}

			var allBlocks [][]byte
			if err := reader.ForEachBlock(func(raw []byte, _ int) error {
				cp := make([]byte, len(raw))
				copy(cp, raw)
				allBlocks = append(allBlocks, cp)
				return nil
			}); err != nil {
				return fmt.Errorf("reading blocks: %w", err)
			}
			totalBlocks := len(allBlocks)
			if totalBlocks == 0 {
				return errors.New("no blocks found in the given directory")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Loaded %d blocks\n", totalBlocks)

			if superblockSize > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Superblock mode: target size=%d bytes\n", superblockSize)
			}

			f, err := os.Create(outFile)
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			defer f.Close()

			var epochManifests []superblock.EpochManifest

			totalDroplets := uint64(0)
			epochID := uint64(0)
			for start := 0; start < totalBlocks; start += epochSize {
				end := start + epochSize
				if end > totalBlocks {
					end = totalBlocks
				}
				epochBlocks := allBlocks[start:end]
				k := len(epochBlocks)

				sourceUnits := epochBlocks
				if superblockSize > 0 {
					supers, blockCounts := superblock.BlocksToSuperblocks(epochBlocks, superblockSize)
					sourceUnits = supers
					epochManifests = append(epochManifests, superblock.EpochManifest{
						EpochID: epochID,
						Manifest: superblock.Manifest{
							TotalBlocks: k,
							TotalSupers: len(supers),
							BlockCounts: blockCounts,
						},
					})
				}
				unitK := len(sourceUnits)

				dist := distribution.NewRobustSoliton(unitK, c, delta)
				params := droplet.NewEpochParams(epochID, uint32(unitK), [32]byte{byte(epochID)})
				enc := droplet.NewEncoder(&params, dist, sourceUnits)

				n := numDroplets
				if n == 0 {
					n = uint64(3 * unitK)
				}

				for did := range n {
					d := enc.Generate(did)
					if err := encode.WriteDroplet(f, &d); err != nil {
						return fmt.Errorf("writing droplet epoch=%d did=%d: %w", epochID, did, err)
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  Epoch %d: blocks [%d,%d) k=%d units=%d droplets=%d\n",
					epochID, start, end, k, unitK, n)
				totalDroplets += n
				epochID++
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d droplets across %d epochs to %s\n",
				totalDroplets, epochID, outFile)

			if superblockSize > 0 && len(epochManifests) > 0 {
				manifestPath := outFile + ".superblock.bin"
				manifestBytes := superblock.SerializeManifests(epochManifests)
				if err := os.WriteFile(manifestPath, manifestBytes, 0644); err != nil {
					return fmt.Errorf("writing superblock manifest: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote superblock manifest to %s\n", manifestPath)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&blocksDir, "blocks-dir", "", "path to the Bitcoin blocks directory (required)")
	_ = cmd.MarkFlagRequired("blocks-dir")
	cmd.Flags().StringVar(&outFile, "out", "droplets.bin", "output file path")
	cmd.Flags().StringVar(&network, "network", "signet", "bitcoin network: mainnet, testnet, signet, regtest")
	cmd.Flags().Float64Var(&c, "c", 0.1, "Robust Soliton c parameter")
	cmd.Flags().Float64Var(&delta, "delta", 0.05, "Robust Soliton delta parameter")
	cmd.Flags().Uint64Var(&numDroplets, "num-droplets", 300, "number of droplets per epoch")
	cmd.Flags().IntVar(&epochSize, "epoch-size", 100, "number of blocks per epoch")
	cmd.Flags().IntVar(&superblockSize, "superblock-size", 4000000, "target superblock size in bytes (0 = disable superblocks)")
	return cmd
}
