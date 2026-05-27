package commands

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/wire"
	"github.com/lucasdbr05/sef-golang/chain"
	"github.com/lucasdbr05/sef-golang/decoder"
	"github.com/lucasdbr05/sef-golang/droplet"
	"github.com/lucasdbr05/sef-golang/encode"
	"github.com/lucasdbr05/sef-golang/superblock"
	"github.com/spf13/cobra"
)

func DecodeCmd() *cobra.Command {
	var dropletsFile string
	var blocksDir string
	var outDir string
	var network string
	var superblockSize int

	cmd := &cobra.Command{
		Use:   "decode",
		Short: "Decode LT-code droplets back into Bitcoin blocks",
		RunE: func(cmd *cobra.Command, args []string) error {
			allDroplets, err := encode.ReadEpochDroplets(dropletsFile)
			if err != nil {
				return fmt.Errorf("reading droplets: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Loaded %d droplets\n", len(allDroplets))

			// Group droplets by EpochID
			epochMap := make(map[uint64][]droplet.Droplet)
			var epochOrder []uint64
			seen := make(map[uint64]bool)
			for _, d := range allDroplets {
				if !seen[d.EpochID] {
					seen[d.EpochID] = true
					epochOrder = append(epochOrder, d.EpochID)
				}
				epochMap[d.EpochID] = append(epochMap[d.EpochID], d)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Found %d epoch(s)\n", len(epochOrder))

			var epochSuperManifests map[uint64]superblock.Manifest
			manifestPath := dropletsFile + ".superblock.bin"
			if superblockSize > 0 || fileExists(manifestPath) {
				data, err := os.ReadFile(manifestPath)
				if err != nil {
					return fmt.Errorf("reading superblock manifest %s: %w", manifestPath, err)
				}
				epochs, err := superblock.DeserializeManifests(data)
				if err != nil {
					return fmt.Errorf("deserializing superblock manifest: %w", err)
				}
				epochSuperManifests = make(map[uint64]superblock.Manifest, len(epochs))
				for _, em := range epochs {
					epochSuperManifests[em.EpochID] = em.Manifest
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Loaded superblock manifests for %d epoch(s)\n", len(epochSuperManifests))
			}

			var headers []wire.BlockHeader
			if blocksDir != "" {
				magic, err := parseNetwork(network)
				if err != nil {
					return err
				}
				reader, err := chain.NewBlkDatReaderAutoXor(blocksDir, magic)
				if err != nil {
					return fmt.Errorf("initializing chain reader: %w", err)
				}
				if err := reader.ForEachBlock(func(raw []byte, _ int) error {
					blk, err := chain.ParseBlock(raw)
					if err != nil {
						return err
					}
					headers = append(headers, blk.Header)
					return nil
				}); err != nil {
					return fmt.Errorf("reading headers: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Loaded %d trusted headers for verification\n", len(headers))
			}

			if outDir != "" {
				if err := os.MkdirAll(outDir, 0755); err != nil {
					return fmt.Errorf("creating output dir: %w", err)
				}
			}

			totalDecoded := 0
			totalExpected := 0
			globalBlockOffset := 0

			for _, eid := range epochOrder {
				drops := epochMap[eid]

				sbManifest, hasSuperblocks := epochSuperManifests[eid]

				var k int
				if hasSuperblocks {
					k = sbManifest.TotalSupers
				} else {
					var maxIdx uint32
					for _, d := range drops {
						for _, idx := range d.Indices {
							if idx > maxIdx {
								maxIdx = idx
							}
						}
					}
					k = int(maxIdx) + 1
				}

				var verifier decoder.BlockVerifier = decoder.AcceptAllVerifier{}
				if len(headers) > 0 {
					if hasSuperblocks {
						start := globalBlockOffset
						end := start + sbManifest.TotalBlocks
						if end > len(headers) {
							end = len(headers)
						}
						epochHeaders := headers[start:end]
						verifier = decoder.NewSuperblockBitcoinVerifier(epochHeaders, sbManifest.BlockCounts)
					} else {
						start := globalBlockOffset
						end := start + k
						if end > len(headers) {
							end = len(headers)
						}
						epochHeaders := headers[start:end]
						verifier = decoder.NewBitcoinBlockVerifier(epochHeaders)
					}
				}

				result := decoder.PeelingDecode(k, drops, verifier)

				var blocks []*[]byte
				var numBlocks int
				if hasSuperblocks {
					blocks = superblock.SuperblocksToBlocks(result.Blocks, sbManifest.BlockCounts, sbManifest.TotalBlocks)
					numBlocks = sbManifest.TotalBlocks
				} else {
					blocks = result.Blocks
					numBlocks = k
				}

				decodedBlocks := 0
				for _, b := range blocks {
					if b != nil {
						decodedBlocks++
					}
				}

				fmt.Fprintf(cmd.OutOrStdout(),
					"  Epoch %d: decoded %d/%d blocks (units %d/%d, stop: %s, verify_failures: %d)\n",
					eid, decodedBlocks, numBlocks, result.DecodedCount, result.K, result.StopReason, result.VerifyFailures)

				totalDecoded += decodedBlocks
				totalExpected += numBlocks

				if outDir != "" {
					for i, blk := range blocks {
						if blk == nil {
							continue
						}
						globalIdx := globalBlockOffset + i
						path := fmt.Sprintf("%s/block_%05d.bin", outDir, globalIdx)
						if err := os.WriteFile(path, *blk, 0644); err != nil {
							return fmt.Errorf("writing block %d: %w", globalIdx, err)
						}
					}
				}

				globalBlockOffset += numBlocks
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Total decoded: %d/%d blocks\n", totalDecoded, totalExpected)
			if totalDecoded < totalExpected {
				return fmt.Errorf("decoding incomplete: %d/%d blocks recovered", totalDecoded, totalExpected)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dropletsFile, "droplets", "droplets.bin", "path to droplets file")
	cmd.Flags().StringVar(&blocksDir, "blocks-dir", "", "optional: path to Bitcoin blocks dir for header verification")
	cmd.Flags().StringVar(&network, "network", "signet", "bitcoin network: mainnet, testnet, signet, regtest")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "optional: directory to write decoded block files")
	cmd.Flags().IntVar(&superblockSize, "superblock-size", 4000000, "superblock target size in bytes (0 = auto-detect from sidecar; ignored if sidecar absent)")
	return cmd
}
