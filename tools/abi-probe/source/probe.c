// SPDX-License-Identifier: MIT

#include <stddef.h>
#include <stdio.h>
#include <sys/ioctl.h>
#include <sound/asound.h>

#define VALUE(name, value) printf(name "=%#llx\n", (unsigned long long)(value))

int main(void) {
	VALUE("control_protocol", SNDRV_CTL_VERSION);
	VALUE("control_card_info_size", sizeof(struct snd_ctl_card_info));
	VALUE("control_components_offset", offsetof(struct snd_ctl_card_info, components));
	VALUE("control_pversion_ioctl", SNDRV_CTL_IOCTL_PVERSION);
	VALUE("control_card_info_ioctl", SNDRV_CTL_IOCTL_CARD_INFO);
	VALUE("pcm_protocol", SNDRV_PCM_VERSION);
	VALUE("pcm_info_size", sizeof(struct snd_pcm_info));
	VALUE("pcm_subdevices_offset", offsetof(struct snd_pcm_info, subdevices_count));
	VALUE("control_pcm_next_ioctl", SNDRV_CTL_IOCTL_PCM_NEXT_DEVICE);
	VALUE("control_pcm_info_ioctl", SNDRV_CTL_IOCTL_PCM_INFO);
	return 0;
}
